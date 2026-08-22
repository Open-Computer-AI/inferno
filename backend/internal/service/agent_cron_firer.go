package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/agent"
	"github.com/Wei-Shaw/sub2api/ent/agentcronfire"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// fireTokenTTL bounds how long a minted fire token is valid. Short-lived
	// like the OAuth access token (oauthAccessTokenTTL) but deliberately its
	// OWN constant, not a shared one -- a fire token authorizes a single POST
	// to one callback URL, not general API access, and the two are free to
	// diverge without either caller noticing.
	fireTokenTTL = 5 * time.Minute

	// fireHTTPTimeout bounds one callback POST. Long enough for a booting
	// agent's gateway to answer 503 (the retryable "not ready yet" case this
	// whole contract exists for), short enough that one wedged agent cannot
	// starve the caller — RehydrateOnBoot fires every armed row's callback
	// serially inline in the timer goroutine go-zero hands it, so a stuck
	// dial here would only ever block that one row's own timer, never the
	// wheel itself (each SetTimer runs its own goroutine).
	fireHTTPTimeout = 15 * time.Second

	// fireLastErrorMaxLen mirrors ent/schema/agent_cron_fire.go's
	// last_error MaxLen(500) -- truncated here rather than left to the
	// database driver to reject the whole UPDATE over an oversized error
	// string, which would turn "record why the callback failed" into "fail
	// to record the retry at all".
	fireLastErrorMaxLen = 500

	// fireRetryBackoffBase/Max bound the re-arm delay after a retryable
	// (non-2xx) response: attempts*30s, capped at 10 minutes. Deliberately
	// well inside TimingWheelService's ~1 hour capacity (1s tick * 3600
	// slots, timing_wheel_service.go) so a retry can never silently fail to
	// schedule the way an arbitrarily-far-future fire_at could.
	fireRetryBackoffBase = 30 * time.Second
	fireRetryBackoffMax  = 10 * time.Minute
)

// cronTimingWheel is the narrow slice of *TimingWheelService this file
// needs: Schedule to arm a (re-)fire, Cancel to drop a stale one. A real
// *TimingWheelService satisfies this structurally (timing_wheel_service.go);
// tests substitute a counting fake so RehydrateOnBoot's contract -- "exactly
// these rows get scheduled" -- can be asserted without waiting on go-zero's
// real 1-second tick.
type cronTimingWheel interface {
	Schedule(name string, delay time.Duration, fn func())
	Cancel(name string)
}

// AgentCronFirer is the thing that actually fires Chronos one-shots: it
// mints a short-lived RS256 "fire" token, POSTs it to the armed row's
// callback_url, and applies the response contract cited to
// hermes_cli/web_routers/cron.py's own docstring (read-only client repo):
// non-2xx is retryable (stays armed, attempts++, last_error recorded,
// re-armed with backoff); any 2xx is terminal (state -> fired, never
// retried) because a 200 also covers "the agent no longer has this job" --
// retrying an intentionally absent fire is the bug that contract guards
// against.
//
// It talks to dbent.AgentCronFire directly rather than through
// AgentCronService, the same "no repository layer for this table" choice
// agent_cron.go itself documents -- and for an additional reason here:
// AgentCronService's surface (Provision/Cancel/ListArmed) is deliberately
// scoped to ONE calling agent (ruling T4-1/T4-2), because that is the
// isolation boundary an agent's own request needs. RehydrateOnBoot is the
// opposite shape -- it must see every agent's armed rows at once on a single
// boot -- so it queries the table directly rather than fan out ListArmed
// across every agent row that exists.
//
// ent/schema/agent_cron_fire.go's own doc comment states the invariant this
// whole type exists to uphold: "This table is the TRUTH. TimingWheelService
// is in-memory, so a restart drops every pending timer; rows are rehydrated
// into the wheel on boot. Nothing errors when that is broken -- the work
// simply never happens." RehydrateOnBoot is that rehydration.
type AgentCronFirer struct {
	client *dbent.Client
	keySvc *OAuthKeyService
	wheel  cronTimingWheel

	// issuer is the `iss` claim stamped on every minted fire token --
	// deliberately the SAME cfg.Server.FrontendURL every other OAuth-issued
	// token in this server stamps (see NewOAuthTokenService's doc comment),
	// so a fire token verifies against the identical JWKS an agent already
	// trusts for its own access token, and normalised the identical way
	// (TrimSpace then TrimRight "/") for the identical reason: one trailing
	// slash here would silently break every callback the exact same way it
	// once broke the whole device grant.
	issuer string

	httpClient *http.Client

	// now is this service's only clock, matching every other service in this
	// package's now-as-a-field seam (e.g. OAuthTokenService.now,
	// AgentRegistryService.now) -- production never replaces it.
	now func() time.Time
}

// NewAgentCronFirer constructs the firer. issuer is normalised identically to
// NewOAuthTokenService's own issuer handling; see AgentCronFirer.issuer.
func NewAgentCronFirer(client *dbent.Client, keySvc *OAuthKeyService, wheel cronTimingWheel, issuer string) *AgentCronFirer {
	return &AgentCronFirer{
		client:     client,
		keySvc:     keySvc,
		wheel:      wheel,
		issuer:     strings.TrimRight(strings.TrimSpace(issuer), "/"),
		httpClient: &http.Client{Timeout: fireHTTPTimeout},
		now:        time.Now,
	}
}

// mintFireToken signs an RS256 JWT authorizing exactly one fire callback.
// Modeled on OAuthTokenService.mintAccessToken (oauth_token_service.go):
// keySvc.Active(ctx), jwt.NewWithClaims(SigningMethodRS256, ...),
// tok.Header["kid"] = key.Kid, tok.SignedString(key.Private) -- and, like
// that function, refuses to mint with a blank issuer (ErrIssuerNotConfigured,
// shared with the access-token mint: an unverifiable token is worse than no
// token in both cases, for the identical reason).
//
// aud is the owning agent's public_id (its own OAuth client_id --
// ent/schema/agent.go's doc comment), and purpose is hardcoded to
// "cron_fire" -- THE claim this whole type exists to add. Cited to
// plugins/cron_providers/chronos/verify.py:140 (read-only client repo):
// without it, an ordinary agent access token -- which already carries a
// valid aud/exp/iss for that same agent -- would be replayable against
// /api/cron/fire by anyone holding one, since every other claim an access
// token and a fire token share would verify identically.
func (f *AgentCronFirer) mintFireToken(ctx context.Context, aud, jobID string) (string, error) {
	// Refuse rather than mint an unverifiable token -- see
	// OAuthTokenService.ErrIssuerNotConfigured, reused here rather than a
	// second sentinel because the failure and its remedy (configure
	// server.frontend_url) are identical for both mints.
	if f.issuer == "" {
		return "", ErrIssuerNotConfigured
	}

	key, err := f.keySvc.Active(ctx)
	if err != nil {
		return "", err
	}

	now := f.now()
	claims := jwt.MapClaims{
		"iss":     f.issuer,
		"aud":     aud,
		"purpose": "cron_fire",
		"job_id":  jobID,
		"iat":     now.Unix(),
		"exp":     now.Add(fireTokenTTL).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = key.Kid

	signed, err := tok.SignedString(key.Private)
	if err != nil {
		return "", fmt.Errorf("agent cron firer: sign fire token: %w", err)
	}
	return signed, nil
}

// fireWheelKey names one fire row's timer inside the timing wheel, shared by
// RehydrateOnBoot's initial arm and FireNow's retry re-arm so a retry
// replaces (rather than duplicates) the row's own outstanding timer.
func fireWheelKey(fireID int64) string {
	return fmt.Sprintf("agent_cron_fire:%d", fireID)
}

// fireCallbackPayload is the JSON body POSTed to callback_url. job_id and
// schedule_id let the agent's own handler correlate the call without relying
// solely on the URL it registered; fire_at is RFC3339 for the same reason
// AgentCronService.ArmedFireView re-serializes it that way rather than
// passing the driver's raw formatting through.
type fireCallbackPayload struct {
	JobID      string `json:"job_id"`
	ScheduleID string `json:"schedule_id"`
	FireAt     string `json:"fire_at"`
}

// FireNow mints a fire token for fireID's row and POSTs it to that row's
// callback_url, then applies the response contract (see AgentCronFirer's
// doc comment). A row that is not (or no longer) "armed" is a silent no-op
// -- defense in depth against a race between a cancel/prior-fire and an
// already-scheduled wheel timer, not the primary guard (RehydrateOnBoot's
// own query already filters to state=armed).
func (f *AgentCronFirer) FireNow(ctx context.Context, fireID int64) error {
	row, err := f.client.AgentCronFire.Get(ctx, fireID)
	if err != nil {
		if dbent.IsNotFound(err) {
			// Nothing left to fire -- e.g. a row deleted out from under an
			// already-armed timer. Not an error: there is no credential to
			// mint and no callback to make.
			return nil
		}
		return fmt.Errorf("agent cron firer: load fire %d: %w", fireID, err)
	}
	if row.State != agentcronfire.StateArmed {
		return nil
	}

	owner, err := f.client.Agent.Query().Where(agent.IDEQ(row.AgentRowID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			// The owning agent row is gone (deleted, not merely revoked --
			// ResolveOwnedAgentRowID already excludes revoked-but-present
			// rows upstream, at provision/cancel time). There is no aud to
			// mint against and nowhere this fire could ever legitimately be
			// authorized for, so mark it fired rather than retry forever.
			return f.markFired(ctx, row.ID)
		}
		return fmt.Errorf("agent cron firer: load owner agent %d for fire %d: %w", row.AgentRowID, row.ID, err)
	}

	token, err := f.mintFireToken(ctx, owner.PublicID, row.JobID)
	if err != nil {
		return f.markRetry(ctx, row, fmt.Sprintf("mint fire token: %v", err))
	}

	body, err := json.Marshal(fireCallbackPayload{
		JobID:      row.JobID,
		ScheduleID: row.ScheduleID,
		FireAt:     row.FireAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		// Unreachable for this fixed, all-string struct, but a marshal
		// failure is an internal fault, not the callback's fault -- do not
		// spend an attempt recording it as the agent's error.
		return fmt.Errorf("agent cron firer: encode fire payload for %d: %w", row.ID, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, row.CallbackURL, bytes.NewReader(body))
	if err != nil {
		return f.markRetry(ctx, row, fmt.Sprintf("build callback request: %v", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		// A network-level failure (gateway unreachable, DNS, timeout, ...)
		// is the SAME retryable case a 503 is -- the agent's own docstring
		// contract is about "is this callback attempt not going to land",
		// and a transport error answers that question identically to a
		// non-2xx status.
		return f.markRetry(ctx, row, fmt.Sprintf("callback request: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Any 2xx is terminal, INCLUDING "job not found" — see this type's
		// doc comment for why retrying an intentionally absent fire is the
		// bug this branch guards against, not a missed retry opportunity.
		return f.markFired(ctx, row.ID)
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, fireLastErrorMaxLen))
	return f.markRetry(ctx, row, fmt.Sprintf("callback answered %d: %s", resp.StatusCode, respBody))
}

// markFired marks fireID's row terminally fired and drops any outstanding
// retry timer for it, so a race between a slow prior retry timer and this
// call cannot double-POST a row this call just terminated.
func (f *AgentCronFirer) markFired(ctx context.Context, fireID int64) error {
	if _, err := f.client.AgentCronFire.UpdateOneID(fireID).
		SetState(agentcronfire.StateFired).
		Save(ctx); err != nil {
		return fmt.Errorf("agent cron firer: mark fire %d fired: %w", fireID, err)
	}
	f.wheel.Cancel(fireWheelKey(fireID))
	return nil
}

// markRetry records a failed attempt and re-arms fireID's row with backoff,
// per the "non-2xx is retryable" half of the response contract: the row
// stays "armed", attempts is incremented, and last_error is recorded --
// never left at its prior value, or an operator reading the row after N
// failures would see only the FIRST failure's cause, not the most recent.
func (f *AgentCronFirer) markRetry(ctx context.Context, row *dbent.AgentCronFire, lastErr string) error {
	attempts := row.Attempts + 1
	if _, err := f.client.AgentCronFire.UpdateOneID(row.ID).
		SetAttempts(attempts).
		SetLastError(truncateFireError(lastErr)).
		Save(ctx); err != nil {
		return fmt.Errorf("agent cron firer: record retry for fire %d: %w", row.ID, err)
	}

	f.wheel.Schedule(fireWheelKey(row.ID), fireRetryBackoff(attempts), func() {
		if err := f.FireNow(context.Background(), row.ID); err != nil {
			logger.LegacyPrintf("service.agent_cron_firer", "[AgentCronFirer] retry fire %d failed: %v", row.ID, err)
		}
	})
	return nil
}

// fireRetryBackoff is attempts*fireRetryBackoffBase, capped at
// fireRetryBackoffMax -- see those constants' doc comment for why the cap
// stays well inside the timing wheel's real capacity.
func fireRetryBackoff(attempts int) time.Duration {
	backoff := time.Duration(attempts) * fireRetryBackoffBase
	if backoff > fireRetryBackoffMax {
		return fireRetryBackoffMax
	}
	return backoff
}

// truncateFireError bounds an error string to fireLastErrorMaxLen, matching
// the last_error column's own MaxLen(500) (ent/schema/agent_cron_fire.go) --
// see that constant's doc comment for why this must clip rather than let the
// UPDATE itself fail.
func truncateFireError(s string) string {
	if len(s) <= fireLastErrorMaxLen {
		return s
	}
	return s[:fireLastErrorMaxLen]
}

// RehydrateOnBoot re-arms every currently ARMED fire into the timing wheel.
// Must run once at server boot, after the timing wheel itself is
// constructed: TimingWheelService is in-memory (timing_wheel_service.go), so
// a restart drops every pending timer, and agent_cron_fires is the truth
// (ent/schema/agent_cron_fire.go's own doc comment) -- this is that
// rehydration. Nothing else in this codebase errors when it does not run;
// the arming simply never happens and a fire silently never fires, which is
// exactly why it has its own test (TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes).
//
// Only state=armed rows are considered -- a "fired" or "cancelled" row must
// NEVER be re-armed on rehydrate, matching FireNow's own guard for the same
// invariant via a different path (a stale in-memory timer that survives a
// state change within one process's lifetime, vs. a restart that drops the
// timer but not the row).
//
// A fire already overdue while the server was down (fire_at in the past) is
// armed with a zero delay rather than skipped or delayed further -- it fires
// once, immediately, rather than being silently dropped for having missed
// its original moment.
func (f *AgentCronFirer) RehydrateOnBoot(ctx context.Context) (int, error) {
	rows, err := f.client.AgentCronFire.Query().
		Where(agentcronfire.StateEQ(agentcronfire.StateArmed)).
		Order(dbent.Asc(agentcronfire.FieldID)).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("agent cron firer: rehydrate: query armed fires: %w", err)
	}

	now := f.now()
	for _, row := range rows {
		delay := row.FireAt.Sub(now)
		if delay < 0 {
			delay = 0
		}
		fireID := row.ID
		f.wheel.Schedule(fireWheelKey(fireID), delay, func() {
			if err := f.FireNow(context.Background(), fireID); err != nil {
				logger.LegacyPrintf("service.agent_cron_firer", "[AgentCronFirer] rehydrated fire %d failed: %v", fireID, err)
			}
		})
	}
	return len(rows), nil
}
