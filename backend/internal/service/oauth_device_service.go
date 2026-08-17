package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthdeviceauthorization"
)

// UserCodeAlphabet deliberately omits 0/O and 1/I/L: the code is read aloud
// and typed by a human.
const UserCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

const (
	deviceCodeTTL      = 15 * time.Minute
	devicePollInterval = 5

	// userCodeCollisionRetries bounds how many times createWithUniqueCodes
	// regenerates BOTH device_code and user_code before giving up, on a
	// unique-constraint collision on either column. user_code is an
	// 8-character code drawn from a 31-character alphabet (~8.5e11
	// combinations) and device_code is 256 bits of crypto/rand, so a
	// collision on the first attempt is already astronomically unlikely;
	// this only exists so a freak collision surfaces as a retry instead of
	// a raw constraint-violation 500 for an innocent caller.
	userCodeCollisionRetries = 5
)

var (
	ErrDeviceCodeNotFound = errors.New("device code not found")
	ErrDeviceCodeExpired  = errors.New("device code expired")

	// ErrPortalNotConfigured is returned by RequestCode when the service was
	// constructed with an empty portalBaseURL. A device flow that emits
	// verification_uri: "/device" (a relative URL) hands the CLI a link
	// nothing can open, so refuse the request loudly instead of returning a
	// half-formed grant.
	ErrPortalNotConfigured = errors.New("oauth device flow: portal base URL not configured")
)

// DeviceCodeGrant is the RFC 8628 device authorization response.
type DeviceCodeGrant struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int
	Interval                int
}

// OAuthDeviceService owns the RFC 8628 device authorization flow: a headless
// CLI requests a device_code/user_code pair, a human approves it in a
// browser via the user_code, and the CLI polls (Task 5) until approved.
type OAuthDeviceService struct {
	entClient     *dbent.Client
	clientSvc     *OAuthClientService
	portalBaseURL string
}

// NewOAuthDeviceService constructs the device-flow service. portalBaseURL is
// the browser-facing base URL used to build verification_uri
// ("{portal}/device") and verification_uri_complete; pass
// cfg.Server.FrontendURL. An empty portalBaseURL is accepted here (so the
// service can be constructed even in deployments that never call
// RequestCode) but RequestCode itself refuses to run without it — see
// ErrPortalNotConfigured.
func NewOAuthDeviceService(entClient *dbent.Client, portalBaseURL string) *OAuthDeviceService {
	return &OAuthDeviceService{
		entClient:     entClient,
		clientSvc:     NewOAuthClientService(entClient),
		portalBaseURL: strings.TrimRight(portalBaseURL, "/"),
	}
}

func randomUserCode() (string, error) {
	out := make([]byte, 0, 9)
	for i := 0; i < 8; i++ {
		if i == 4 {
			out = append(out, '-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(UserCodeAlphabet))))
		if err != nil {
			return "", err
		}
		out = append(out, UserCodeAlphabet[n.Int64()])
	}
	return string(out), nil
}

// RequestCode starts a device-flow authorization for a registered client.
// clientID must belong to an already-registered service.OAuthClient — an
// unknown client_id is rejected rather than silently starting a flow for an
// identity nobody issued.
func (s *OAuthDeviceService) RequestCode(ctx context.Context, clientID, scope string) (*DeviceCodeGrant, error) {
	if s.portalBaseURL == "" {
		slog.Error("oauth: device code request rejected, portal base URL not configured", "client_id", clientID)
		return nil, ErrPortalNotConfigured
	}

	if _, err := s.clientSvc.ByClientID(ctx, clientID); err != nil {
		return nil, fmt.Errorf("unknown client_id %q: %w", clientID, err)
	}

	row, err := s.createWithUniqueCodes(ctx, clientID, scope)
	if err != nil {
		return nil, err
	}

	verifyURI := s.portalBaseURL + "/device"
	complete := verifyURI + "?user_code=" + url.QueryEscape(row.UserCode)

	return &DeviceCodeGrant{
		DeviceCode:              row.DeviceCode,
		UserCode:                row.UserCode,
		VerificationURI:         verifyURI,
		VerificationURIComplete: complete,
		ExpiresIn:               int(deviceCodeTTL.Seconds()),
		Interval:                devicePollInterval,
	}, nil
}

// createWithUniqueCodes inserts the device authorization row, regenerating
// BOTH device_code and user_code (device_code and user_code are each
// schema-Unique()) on a constraint collision rather than failing the whole
// request outright. A collision could in principle come from either column,
// and re-deriving only one of them would retry with the same losing value on
// the other, so every attempt draws fresh entropy for both.
func (s *OAuthDeviceService) createWithUniqueCodes(ctx context.Context, clientID, scope string) (*dbent.OAuthDeviceAuthorization, error) {
	var lastErr error
	for attempt := 0; attempt < userCodeCollisionRetries; attempt++ {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("device code entropy: %w", err)
		}
		deviceCode := hex.EncodeToString(raw)

		userCode, err := randomUserCode()
		if err != nil {
			return nil, fmt.Errorf("user code: %w", err)
		}

		row, err := s.entClient.OAuthDeviceAuthorization.Create().
			SetDeviceCode(deviceCode).
			SetUserCode(userCode).
			SetClientID(clientID).
			SetScope(scope).
			SetStatus("pending").
			SetExpiresAt(time.Now().Add(deviceCodeTTL)).
			Save(ctx)
		if err == nil {
			return row, nil
		}
		if !dbent.IsConstraintError(err) {
			return nil, fmt.Errorf("persist device authorization: %w", err)
		}
		lastErr = err
		slog.Warn("oauth: device authorization insert collided, retrying", "client_id", clientID, "attempt", attempt+1)
	}
	return nil, fmt.Errorf("persist device authorization: exhausted retries: %w", lastErr)
}

func (s *OAuthDeviceService) byDeviceCode(ctx context.Context, deviceCode string) (*dbent.OAuthDeviceAuthorization, error) {
	row, err := s.entClient.OAuthDeviceAuthorization.Query().
		Where(oauthdeviceauthorization.DeviceCode(deviceCode)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, ErrDeviceCodeNotFound
	}
	return row, err
}

func (s *OAuthDeviceService) setStatusByUserCode(ctx context.Context, userCode, status string, userID *int64) error {
	row, err := s.entClient.OAuthDeviceAuthorization.Query().
		Where(oauthdeviceauthorization.UserCode(userCode)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return ErrDeviceCodeNotFound
	}
	if err != nil {
		return fmt.Errorf("query device authorization: %w", err)
	}
	if time.Now().After(row.ExpiresAt) {
		return ErrDeviceCodeExpired
	}

	upd := row.Update().SetStatus(status)
	if userID != nil {
		upd = upd.SetApprovedUserID(*userID)
	}
	if _, err := upd.Save(ctx); err != nil {
		return fmt.Errorf("update device authorization: %w", err)
	}
	return nil
}

// Approve marks a device authorization approved by the given user.
func (s *OAuthDeviceService) Approve(ctx context.Context, userCode string, userID int64) error {
	return s.setStatusByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)), "approved", &userID)
}

// Deny marks a device authorization denied.
func (s *OAuthDeviceService) Deny(ctx context.Context, userCode string) error {
	return s.setStatusByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)), "denied", nil)
}
