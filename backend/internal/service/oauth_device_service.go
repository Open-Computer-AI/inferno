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

// Device authorization lifecycle. `expired` doubles as "consumed": a device
// code is single-use, and ExchangeDeviceCode marks a redeemed row `expired`
// because RFC 8628 has no distinct "already used" error code.
const (
	DeviceStatusPending  = "pending"
	DeviceStatusApproved = "approved"
	DeviceStatusDenied   = "denied"
	DeviceStatusExpired  = "expired"
)

var (
	ErrDeviceCodeNotFound = errors.New("device code not found")
	ErrDeviceCodeExpired  = errors.New("device code expired")

	// ErrDeviceCodeNotPending is returned when a device authorization exists
	// and has not timed out, but has already left `pending` — it was approved,
	// denied, or redeemed. Distinct from ErrDeviceCodeExpired so the approval
	// screen can say "that decision was already made" rather than mislabelling
	// it a timeout.
	ErrDeviceCodeNotPending = errors.New("device code is no longer pending")

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

	// Scope is validated BEFORE the client lookup, and the order is load-bearing.
	// Validating after it would mean a caller sending a deliberately-bogus scope
	// gets invalid_scope when the client exists and invalid_client when it does
	// not — turning the difference between the two errors into a client_id
	// existence oracle, and defeating the collapse below.
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}

	if _, err := s.clientSvc.UsableByClientID(ctx, clientID); err != nil {
		// Unknown and not-usable (revoked) collapse into one wrapped error on
		// purpose — the handler renders both as a single "invalid_client", so
		// this endpoint is not a client_id existence oracle.
		return nil, fmt.Errorf("client_id %q not usable: %w", clientID, err)
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
			SetStatus(DeviceStatusPending).
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

// setStatusByUserCode moves a device authorization out of `pending` — and out
// of `pending` ONLY.
//
// The status guard is load-bearing, not defensive tidiness. Without it every
// status was writable for as long as the 15-minute expiry allowed, so both of
// these were legal:
//
//   - expired(consumed) → approved. ExchangeDeviceCode consumes an approved row
//     by setting status="expired". Re-entering the same user_code and clicking
//     Approve again RE-ARMED that row, letting whoever holds the device_code
//     redeem a SECOND access token and a second 30-day refresh family from a
//     single human approval. A person who approves, sees nothing happen, and
//     approves again — the most natural thing to do — triggers exactly this.
//   - denied → approved. A deliberate Deny could be silently overturned by
//     anyone who could get the code re-submitted.
//
// The guard is a status-predicated UPDATE rather than a Go-side check on the
// row read above, so it is also atomic: two concurrent approvals of the same
// code cannot both observe `pending` and both write. Same CAS shape as
// ExchangeDeviceCode's consuming update.
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

	upd := s.entClient.OAuthDeviceAuthorization.Update().
		Where(
			oauthdeviceauthorization.ID(row.ID),
			oauthdeviceauthorization.Status(DeviceStatusPending),
		).
		SetStatus(status)
	if userID != nil {
		upd = upd.SetApprovedUserID(*userID)
	}
	affected, err := upd.Save(ctx)
	if err != nil {
		return fmt.Errorf("update device authorization: %w", err)
	}
	if affected == 0 {
		// Already approved, denied, or consumed. Reported distinctly from
		// "expired" so the caller can tell the human their decision was already
		// recorded rather than implying their code timed out.
		return ErrDeviceCodeNotPending
	}
	return nil
}

// PendingAuthorization is what the human is being asked to approve: which
// client is asking, and for what.
type PendingAuthorization struct {
	// ClientName is the client's display label — the docker-style
	// adjective_noun a self-hosted agent was registered with, or "hermes-cli"
	// for the first-party CLI. It is user-supplied for self-hosted clients and
	// must be rendered as text, never as markup.
	ClientName string
	// ClientID is the public client identifier. Shown alongside the name
	// because the name is neither unique nor server-controlled, so it alone
	// cannot distinguish a legitimate agent from one an attacker registered
	// with a convincing label.
	ClientID string
	// Scopes are the individual scope entries requested, already validated
	// against the vocabulary at RequestCode time.
	Scopes []string
	// ExpiresAt is when the code stops being approvable.
	ExpiresAt time.Time
}

// PendingByUserCode resolves a user_code to the authorization awaiting a
// decision, so the approval screen can tell the human WHO is asking and WHAT
// they are asking for.
//
// RFC 8628 §5.4 requires the authorization server to display client and
// authorization information at the verification URI; before this existed the
// screen showed a bare code box and two buttons, which is indistinguishable
// from a legitimate login prompt and is exactly what makes a phished user_code
// dangerous.
//
// Callers MUST require a live panel session before calling this. It returns
// ErrDeviceCodeNotFound for both "no such code" and "not currently pending",
// so it cannot be used to probe which user_codes exist or what state they are
// in.
func (s *OAuthDeviceService) PendingByUserCode(ctx context.Context, userCode string) (*PendingAuthorization, error) {
	normalized := strings.ToUpper(strings.TrimSpace(userCode))
	row, err := s.entClient.OAuthDeviceAuthorization.Query().
		Where(oauthdeviceauthorization.UserCode(normalized)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, ErrDeviceCodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query device authorization: %w", err)
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, ErrDeviceCodeExpired
	}
	if row.Status != DeviceStatusPending {
		return nil, ErrDeviceCodeNotPending
	}

	name := row.ClientID
	if client, cerr := s.clientSvc.ByClientID(ctx, row.ClientID); cerr == nil && client.Name != "" {
		name = client.Name
	} else if cerr != nil {
		// The client row vanished (or the read failed) between RequestCode and
		// now. Fall back to the client_id rather than failing the whole screen:
		// showing the raw identifier is less informative than the label, but
		// showing NOTHING is what this endpoint exists to prevent.
		slog.Warn("oauth: could not resolve client name for pending authorization",
			"client_id", row.ClientID, "error", cerr)
	}

	return &PendingAuthorization{
		ClientName: name,
		ClientID:   row.ClientID,
		Scopes:     ScopeList(row.Scope),
		ExpiresAt:  row.ExpiresAt,
	}, nil
}

// Approve marks a device authorization approved by the given user.
func (s *OAuthDeviceService) Approve(ctx context.Context, userCode string, userID int64) error {
	return s.setStatusByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)), "approved", &userID)
}

// Deny marks a device authorization denied.
func (s *OAuthDeviceService) Deny(ctx context.Context, userCode string) error {
	return s.setStatusByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)), "denied", nil)
}
