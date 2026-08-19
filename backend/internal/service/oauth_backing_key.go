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

// ErrInvalidBackingKeyRequest marks the caller's arguments as unusable: a
// non-positive user id, an empty client_id, or a service with no database
// client.
//
// It exists so those guards can be tested for the reason they name. Without it
// the assertions were satisfied by a second mechanism -- the api_keys.user_id
// foreign key rejects user id 0 anyway, so `require.Error` and "no row was
// written" both held with the guard deleted, and the guard itself was untested.
// A caller also gets a clean signal instead of a misleading "rejected by a
// unique constraint" 500.
var ErrInvalidBackingKeyRequest = errors.New("oauth backing key: invalid request")

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

// ErrOAuthBackingKeyUnmodifiable rejects any attempt to EDIT an OAuth backing
// row through a user-facing key-management path.
//
// Why refuse the whole edit rather than the dangerous columns:
//
//   - group_id is the dangerous one, and it is dangerous in a way the system
//     cannot heal from. apiKey.Group is the only routing input in the entire
//     gateway pipeline -- platform selection, channel pool, model mapping and
//     pricing all read it -- and OAuthBackingKeyService.Resolve only rebinds a
//     backing row's group when that group is MISSING or INACTIVE. Point a
//     backing row at a different *active* group and it stays there forever, so
//     one guessed PUT silently and permanently re-routes and re-prices an
//     agent's inference.
//   - expires_at, status and the IP ACL are all user-reachable bricks. Task 4
//     closed finding F1 by making the IP ACL actually apply to OAuth-backed
//     requests, which means a whitelist edit can now lock an agent out -- and
//     with edits refused there is no way back in, exactly the shape of brick
//     Task 3 closed for Delete.
//   - name is the operator's only marker that the row is agent-backed
//     (backingKeyName), so renaming it degrades the one signal an operator has.
//
// That leaves no edit worth allowing. And the user cannot reach the row through
// the UI at all -- it is filtered out of every listing and every by-id read --
// so any Update that arrives for one came from id guessing, not from a feature.
// An operation no interface can initiate is not a capability being removed.
//
// It is a 403 rather than the 404 the read paths give, matching
// ErrOAuthBackingKeyUndeletable: a write against server-managed state deserves
// an answer that says what happened, while a read deserves the row's
// invisibility.
var ErrOAuthBackingKeyUnmodifiable = infraerrors.Forbidden(
	"OAUTH_BACKING_KEY_UNMODIFIABLE",
	"this API key backs an OAuth agent and is managed by the server; it cannot be edited",
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
// This file therefore has no accessor that yields the secret, blanks
// api_keys.key on every row it returns (redactBackingKeySecret), and strips both
// the text and the value of any create-path error (sanitizeBackingKeyError).
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
		return nil, fmt.Errorf("%w: no database client", ErrInvalidBackingKeyRequest)
	}
	if userID <= 0 {
		return nil, fmt.Errorf("%w: user id %d is not positive", ErrInvalidBackingKeyRequest, userID)
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		// An empty client_id would store NULL, which migration 909's partial
		// index ignores -- every agent of this user would then collapse onto
		// unbounded duplicate rows with no identity rule at all.
		return nil, fmt.Errorf("%w: empty client_id", ErrInvalidBackingKeyRequest)
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
		// Sanitize before the error travels. This branch is the one that gets a
		// BARE *pq.Error, whose exported Detail field carries the freshly
		// generated credential -- see sanitizeBackingKeyError for exactly which
		// serialisers leak it and which do not.
		return nil, fmt.Errorf("oauth backing key: create: %w", sanitizeBackingKeyError(err, secret))
	}

	winner, lookupErr := s.lookup(ctx, userID, clientID)
	if lookupErr != nil {
		return nil, lookupErr
	}
	if winner == nil {
		// A constraint fired but no live row matches the pair.
		//
		// Migration 910 scoped the identity index to deleted_at IS NULL, which
		// removed the case this branch was originally written for (a tombstone
		// holding the slot). It is NOT dead code, but it is now vanishingly
		// rare: the reachable causes are a collision on api_keys.key itself
		// (~2^-256), and the winner's row being deleted between our rejected
		// INSERT and this re-read. Both are reported rather than retried,
		// because a retry loop here would hammer a table under whatever
		// condition produced the anomaly.
		//
		// It is reached deliberately by
		// TestCreatePathErrorNeverCarriesTheCredential_ConstraintError, which is
		// also what pins the sanitizer on this path.
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
// Every consumer of api_keys.key outside key management was checked, and none
// breaks on a blank one:
//
//   - handler/ops_error_logger.go and handler/openai_gateway_handler.go call
//     keyPrefix(apiKey.Key, 8) for ops metadata, which now renders empty for
//     OAuth-backed requests. Intended: an ops log should not carry the leading
//     bytes of a credential the server promised never to surface. Use
//     oauth_client_id if per-agent correlation is wanted.
//   - gateway_usage_billing.go's quota-exhausted hook already guards
//     `p.APIKey.Key != ""` before InvalidateAuthCacheByKey, so it skips. That is
//     correct rather than merely harmless: the auth cache is keyed by a secret
//     presented as a bearer credential, and a backing key is never presented as
//     one, so there is no entry under it to invalidate.
//   - admin_group.go / admin_user.go / api_key_service.go invalidate on rows
//     fetched from the repository, which still carry the real secret. They never
//     see a row that came through here.
//
// CONSTRAINT ON TASK 4: because the row it receives has a blank Key, it must not
// route OAuth-backed requests through APIKeyService.ValidateKey or anything else
// that authenticates by key string. It resolves identity from the token, not
// from the credential.
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
// # What actually leaks, measured
//
// A PostgreSQL unique violation on api_keys.key arrives with the credential on
// the exported pq.Error.Detail field --
// `Key (key)=(sk-...) already exists.` -- verified against real PostgreSQL 18 in
// repository.APIKeyOAuthClientIDSuite.TestPostgresUniqueViolationHidesTheKeyInErrorTextButNotInTheErrorValue.
// lib/pq's Error() renders only Severity and Message, so err.Error() and %+v are
// clean; the value is not.
//
// The two shapes behave differently, and the difference is worth stating exactly
// rather than overstating it:
//
//   - A BARE *pq.Error -- what the non-constraint branch above receives --
//     leaks through everything that walks the value: %#v, json.Marshal,
//     zap.Reflect and zap.Any alike, because pq.Error's fields are exported.
//   - An *ent.ConstraintError wrapping a *pq.Error -- the other branch -- does
//     NOT leak through those: zap.Any takes its `case error` path and calls
//     Error(), %#v prints `wrap:(*pq.Error)(0x...)` as a pointer, and
//     json.Marshal/zap.Reflect yield `{}` because ent.ConstraintError's fields
//     are unexported. The credential is still reachable there, but only by
//     errors.As-ing down to the *pq.Error -- which is exactly what code that
//     branches on PostgreSQL error codes does.
//
// So the value-level leak is direct in one branch and one unwrap away in the
// other. Both are closed the same way.
//
// # The two measures
//
//  1. **Flatten the error value.** Return a plain error, so no driver error
//     escapes for anything downstream to serialise or unwrap. This is the
//     measure that closes the channel; rewriting the message can never reach a
//     credential that is not in the message.
//  2. **Redact the message text.** Defence in depth for a driver that does
//     render DETAIL (pgx formats differently, and lib/pq's rendering is not a
//     contract) and for any wrapper that has already folded the value into a
//     string.
//
// # The one thing flattening must not destroy
//
// context.Canceled and context.DeadlineExceeded are branched on elsewhere in
// this codebase, and a client that hung up is not a failed request: dropping
// their identity would make the caller map a disconnect to a 500. They are
// re-wrapped explicitly. Both are payload-free sentinels, so preserving them
// carries nothing -- errors.As still cannot reach a driver error through the
// result.
//
// Call it only after any errors.Is / dbent.IsConstraintError checks on the
// original: by design nothing else survives for errors.As to find.
func sanitizeBackingKeyError(err error, secret string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%s: %w", msg, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s: %w", msg, context.DeadlineExceeded)
	}
	return errors.New(msg)
}
