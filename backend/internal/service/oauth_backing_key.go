package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ErrNoGroupForOAuthKey reports that no group could be resolved for an OAuth
// backing key. The caller answers 403 with an operator-readable message.
//
// It exists so the "no group" case can never become a returned row with a nil
// Group: apiKey.Group is the only input to platform routing, channel pool
// selection, model mapping and pricing, and a nil one reaches routing and
// panics. Failing here is the cheap failure; returning the row is the expensive
// one.
var ErrNoGroupForOAuthKey = errors.New("oauth backing key: no group configured")

// backingKeyNameMaxLen mirrors api_keys.name's MaxLen(100) (ent/schema/api_key.go).
const backingKeyNameMaxLen = 100

// OAuthBackingKeyService resolves a verified OAuth access token's (user,
// client) pair to the api_keys row the gateway pipeline meters against.
//
// # Why a row has to exist at all
//
// There is no keyless inference path in this codebase. usage_logs.user_id,
// .api_key_id and .account_id are all NOT NULL, usage_logs_api_key_id_fkey
// points at api_keys(id), usage_billing_dedup.api_key_id is NOT NULL and half
// of UNIQUE (request_id, api_key_id), and the quota/rate-limit ledger *is* the
// key row (api_keys.quota_used, usage_5h|1d|7d, window_*_start). So an OAuth
// token is given an internal api_keys row -- one per (user, oauth client) --
// created on first use.
//
// # The security rule that makes this safe
//
// A backing row carries a `key` column, and an api_keys secret is a bearer
// credential that does not expire. The one rule containing that risk: the
// backing key's secret is NEVER returned to anyone, by any endpoint, ever. It
// is a row the server resolves *to*, never a credential the server hands *out*.
// This file therefore has no accessor that yields the secret, and takes care
// (see scrubBackingKeySecret) not to let one escape through an error string.
//
// # Never hard-delete a backing row
//
// usage_logs_api_key_id_fkey is ON DELETE CASCADE. Deleting a backing row
// silently erases that agent's entire usage history. Nothing here deletes one;
// a disabled state is a status change.
type OAuthBackingKeyService struct {
	entClient *dbent.Client
	cfg       *config.Config
}

func NewOAuthBackingKeyService(entClient *dbent.Client, cfg *config.Config) *OAuthBackingKeyService {
	return &OAuthBackingKeyService{entClient: entClient, cfg: cfg}
}

// Resolve returns the backing api_keys row for (userID, clientID), creating it
// on first use. The returned row always has User and Group loaded, because the
// gateway pipeline reads both.
//
// Errors: ErrNoGroupForOAuthKey (wrapped, with the configured name) when the
// group policy resolves to nothing -- the caller's 403. Everything else is an
// infrastructure fault and must surface as a 500; in particular a database
// error while resolving the group is NOT ErrNoGroupForOAuthKey.
func (s *OAuthBackingKeyService) Resolve(ctx context.Context, userID int64, clientID string) (*dbent.APIKey, error) {
	if s == nil || s.entClient == nil {
		return nil, errors.New("oauth backing key: no database client")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("oauth backing key: invalid user id %d", userID)
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		// An empty client_id would store NULL, which migration 909's partial
		// index ignores -- every agent of this user would then collapse onto
		// unbounded duplicate rows with no identity rule at all.
		return nil, errors.New("oauth backing key: empty client_id")
	}

	existing, err := s.lookup(ctx, userID, clientID)
	if err != nil {
		return nil, err
	}
	// Happy path: the row exists and is complete. No group query needed, which
	// keeps steady-state inference at one round trip.
	if existing != nil && existing.Edges.Group != nil {
		return existing, nil
	}

	grp, err := s.policyGroup(ctx)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// The row lost its group (its group was deleted, or an older row
		// predates the policy). Rebind rather than hand back a nil Group.
		if err := s.entClient.APIKey.UpdateOneID(existing.ID).SetGroupID(grp.ID).Exec(ctx); err != nil {
			return nil, fmt.Errorf("oauth backing key: bind group to row %d: %w", existing.ID, err)
		}
		return s.reload(ctx, existing.ID)
	}

	return s.createOrAdoptWinner(ctx, userID, clientID, grp.ID)
}

// createOrAdoptWinner inserts the backing row, or -- if a concurrent caller got
// there first -- adopts the row that concurrent caller created.
//
// Migration 909's partial unique index on (user_id, oauth_client_id) is the
// serialization point. Two simultaneous first requests from one agent both miss
// the lookup and both attempt the INSERT; exactly one succeeds and the other
// gets a unique violation. Turning that violation into a re-read is what stops
// an agent's first two concurrent inference calls from producing one success
// and one 500.
func (s *OAuthBackingKeyService) createOrAdoptWinner(ctx context.Context, userID int64, clientID string, groupID int64) (*dbent.APIKey, error) {
	prefix := ""
	if s.cfg != nil {
		prefix = s.cfg.Default.APIKeyPrefix
	}
	secret, err := GenerateAPIKeySecret(prefix)
	if err != nil {
		return nil, fmt.Errorf("oauth backing key: generate secret: %w", err)
	}

	created, err := s.entClient.APIKey.Create().
		SetUserID(userID).
		SetKey(secret).
		SetName(backingKeyName(clientID)).
		SetOauthClientID(clientID).
		SetGroupID(groupID).
		SetStatus(domain.StatusActive).
		Save(ctx)
	if err == nil {
		return s.reload(ctx, created.ID)
	}

	if !dbent.IsConstraintError(err) {
		// scrub before the error travels: a unique violation on api_keys.key
		// would otherwise carry the freshly generated credential in its DETAIL.
		return nil, fmt.Errorf("oauth backing key: create: %w", scrubBackingKeySecret(err, secret))
	}

	winner, lookupErr := s.lookup(ctx, userID, clientID)
	if lookupErr != nil {
		return nil, lookupErr
	}
	if winner == nil {
		// The index rejected the insert but no *live* row matches. The index
		// does not filter deleted_at, so a soft-deleted backing row still
		// occupies the (user_id, oauth_client_id) slot. That is an operator
		// situation, not a race; report it rather than looping.
		return nil, fmt.Errorf(
			"oauth backing key: insert for user %d client %q was rejected by the (user_id, oauth_client_id) unique index but no live row matches; a soft-deleted backing row still occupies that slot: %w",
			userID, clientID, scrubBackingKeySecret(err, secret))
	}
	if winner.Edges.Group == nil {
		if bindErr := s.entClient.APIKey.UpdateOneID(winner.ID).SetGroupID(groupID).Exec(ctx); bindErr != nil {
			return nil, fmt.Errorf("oauth backing key: bind group to row %d: %w", winner.ID, bindErr)
		}
		return s.reload(ctx, winner.ID)
	}
	return winner, nil
}

// lookup returns the live backing row for the pair, or (nil, nil) when there is
// none. The soft-delete interceptor scopes it to deleted_at IS NULL.
func (s *OAuthBackingKeyService) lookup(ctx context.Context, userID int64, clientID string) (*dbent.APIKey, error) {
	row, err := s.entClient.APIKey.Query().
		Where(
			apikey.UserIDEQ(userID),
			apikey.OauthClientIDEQ(clientID),
		).
		WithUser().
		WithGroup().
		Only(ctx)
	switch {
	case dbent.IsNotFound(err):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("oauth backing key: look up row for user %d client %q: %w", userID, clientID, err)
	}
	if row.Edges.User == nil {
		return nil, fmt.Errorf("oauth backing key: backing row %d has no live owner (user %d)", row.ID, userID)
	}
	return row, nil
}

// reload re-reads a row with User and Group eager-loaded. Create/Update return
// the row without its edges, and the caller contract is that both are present.
func (s *OAuthBackingKeyService) reload(ctx context.Context, id int64) (*dbent.APIKey, error) {
	row, err := s.entClient.APIKey.Query().
		Where(apikey.IDEQ(id)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("oauth backing key: reload row %d: %w", id, err)
	}
	if row.Edges.User == nil {
		return nil, fmt.Errorf("oauth backing key: backing row %d has no live owner", id)
	}
	if row.Edges.Group == nil {
		return nil, fmt.Errorf("oauth backing key: backing row %d has no group after binding", id)
	}
	return row, nil
}

// policyGroup resolves the configured group by name, the same way the
// platform's other default groups are resolved (repository.createGroupIfNotExists
// matches on name + not-soft-deleted).
//
// A missing or unset policy is ErrNoGroupForOAuthKey; a database failure is
// not, because an infrastructure fault must never read as an auth/config
// failure.
func (s *OAuthBackingKeyService) policyGroup(ctx context.Context) (*dbent.Group, error) {
	name := ""
	if s.cfg != nil {
		name = strings.TrimSpace(s.cfg.OAuthBackingKey.GroupName)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: set oauth_backing_key.group_name to the group OAuth agents should bill against", ErrNoGroupForOAuthKey)
	}

	grp, err := s.entClient.Group.Query().
		Where(
			group.NameEQ(name),
			group.StatusEQ(domain.StatusActive),
		).
		Only(ctx)
	switch {
	case dbent.IsNotFound(err):
		return nil, fmt.Errorf("%w: oauth_backing_key.group_name = %q matches no active group", ErrNoGroupForOAuthKey, name)
	case dbent.IsNotSingular(err):
		return nil, fmt.Errorf("%w: oauth_backing_key.group_name = %q matches more than one active group", ErrNoGroupForOAuthKey, name)
	case err != nil:
		return nil, fmt.Errorf("oauth backing key: resolve group %q: %w", name, err)
	}
	return grp, nil
}

// backingKeyName labels the row as agent-backed so an operator reading the
// table can tell it apart from a key a user made. Clamped to api_keys.name's
// MaxLen(100).
func backingKeyName(clientID string) string {
	name := "OAuth agent " + clientID
	if len(name) > backingKeyNameMaxLen {
		name = name[:backingKeyNameMaxLen]
	}
	return name
}

// scrubBackingKeySecret removes a freshly generated credential from an error
// before it travels anywhere a log or an API response can see it.
//
// Postgres puts the offending value in a unique violation's DETAIL line, so an
// INSERT that collided on api_keys.key would otherwise carry the secret upward
// verbatim. Call it only after any errors.Is/IsConstraintError checks: it
// returns a flat error and does not preserve wrapping.
func scrubBackingKeySecret(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, secret) {
		return err
	}
	return errors.New(strings.ReplaceAll(msg, secret, "[redacted]"))
}
