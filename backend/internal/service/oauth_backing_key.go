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
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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

// ErrOAuthBackingKeyUndeletable rejects any attempt to delete an OAuth backing
// row through a user-facing path.
//
// The row is owned by the user, so ownership alone is not a sufficient
// authorization check: it IS that agent's quota and rate-limit ledger, and
// usage_logs_api_key_id_fkey (ON DELETE CASCADE) hangs the agent's entire usage
// history off it. Even a soft delete is destructive, because migration 909's
// identity index and ent's soft-delete interceptor disagree about tombstones
// (migration 910 fixes the recoverability half of that; this error is the half
// that stops the state being reached at all).
var ErrOAuthBackingKeyUndeletable = infraerrors.Forbidden(
	"OAUTH_BACKING_KEY_UNDELETABLE",
	"this API key backs an OAuth agent and cannot be deleted; revoke the agent's authorization instead",
)

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
	// Happy path: the row exists and its group is usable. No group query is
	// issued, which matters for two reasons. It keeps steady-state inference at
	// one round trip -- and, without it, EVERY steady-state request would fall
	// through to a create whose INSERT the identity index refuses, producing a
	// dead tuple per request on the hottest path in the system. It also means
	// an already-provisioned agent keeps serving if an operator later mistypes
	// or removes the group policy: a config typo must not 403 every agent that
	// already has a backing row. (TestResolveKeepsServingAnAlreadyProvisionedAgentAfterThePolicyBreaks
	// pins that; it is a deliberate choice for continuity, not an accident.)
	//
	// An inactive group is treated like a missing one and rebound below: the
	// status is already loaded on the edge, so checking it is free, and the
	// alternative -- agents quietly billing through a group the operator has
	// switched off -- is the surprising behaviour.
	if existing != nil && existing.Edges.Group != nil && existing.Edges.Group.Status == domain.StatusActive {
		return existing, nil
	}

	grp, err := s.policyGroup(ctx)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// The row has no usable group: its group was soft-deleted (the
		// interceptor filters it, so the edge comes back nil), or deactivated,
		// or the row predates the policy. Rebind rather than hand back a row
		// routing cannot use.
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
		return nil, fmt.Errorf("oauth backing key: create: %w", sanitizeBackingKeyError(err, secret))
	}

	winner, lookupErr := s.lookup(ctx, userID, clientID)
	if lookupErr != nil {
		return nil, lookupErr
	}
	if winner == nil {
		// A constraint fired but no live row matches the pair. Since migration
		// 910 scoped the identity index to "deleted_at IS NULL", a tombstone no
		// longer holds the slot, so the remaining reachable cause is a
		// collision on api_keys.key itself -- which is exactly the case whose
		// Postgres DETAIL line carries the freshly generated credential, hence
		// the scrub. Report rather than loop.
		return nil, fmt.Errorf(
			"oauth backing key: insert for user %d client %q was rejected by a unique constraint but no live backing row matches the pair: %w",
			userID, clientID, sanitizeBackingKeyError(err, secret))
	}
	if winner.Edges.Group == nil || winner.Edges.Group.Status != domain.StatusActive {
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
	return redactBackingKeySecret(row), nil
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
	return redactBackingKeySecret(row), nil
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

// redactBackingKeySecret blanks api_keys.key on a row about to leave this
// service.
//
// The design rule is that a backing key's secret is never handed out, by any
// endpoint, ever -- and a struct field is a hand-out. *dbent.APIKey.String()
// writes "key=<secret>" verbatim (ent/apikey.go) and the field is
// `json:"key,omitempty"`, so one careless %v, one c.JSON of the row, or one gin
// error dump that serializes the context is a credential leak. Task 4 puts this
// exact struct into the gin context, so the blast radius is a single format
// verb. Emptying the field means String() and JSON marshalling have nothing to
// leak.
//
// This is done here rather than with ent's .Sensitive() on the schema field,
// because ordinary key creation must still return its secret to its owner
// exactly once; marking the field sensitive globally would break that. The
// redaction belongs at this boundary, which is the only one that must never
// emit a secret.
//
// Nothing in the gateway pipeline reads api_keys.key functionally -- the only
// consumers are handler/ops display paths calling keyPrefix(apiKey.Key, 8),
// which now render empty for OAuth-backed requests. That is the intended
// trade: an ops log should not carry the leading bytes of a credential the
// server promised never to surface.
// It is applied in exactly two places -- lookup and reload, the only two
// functions in this file that produce a row -- so no return path can forget it,
// and there is no second redundant guard to make a test pass for the wrong
// reason.
func redactBackingKeySecret(row *dbent.APIKey) *dbent.APIKey {
	if row == nil {
		return nil
	}
	row.Key = ""
	return row
}

// sanitizeBackingKeyError makes an error from the create path safe to return,
// log or serialise, by two separate measures.
//
// **It flattens the error value.** This is the one that matters, and it is not
// what the original version did. A PostgreSQL unique violation on api_keys.key
// arrives as a *pq.Error whose exported Detail field reads
// `Key (key)=(sk-...) already exists.` -- verified against real PostgreSQL 18 in
// repository.APIKeyOAuthClientIDSuite.TestPostgresUniqueViolationHidesTheKeyInErrorTextButNotInTheErrorValue.
// lib/pq's Error() renders only Severity and Message, so the credential is
// absent from err.Error() and %+v, but present on the value that every wrapped
// chain carries upward. Anything that serialises an error structurally --
// zap.Any("err", err), %#v, a JSON error reporter -- prints it in full. Rewriting
// the message can never reach that; dropping the chain can, so this returns a
// flat errors.New and the driver error does not escape.
//
// **It redacts the message text.** Defence in depth for a driver that does
// render DETAIL (pgx formats differently, and lib/pq's rendering is not a
// contract), and for any wrapper that has already folded the value into a
// string.
//
// Call it only after any errors.Is / dbent.IsConstraintError checks: by design
// nothing survives for errors.As to find.
func sanitizeBackingKeyError(err error, secret string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	return errors.New(msg)
}
