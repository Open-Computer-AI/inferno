package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthclient"
)

const (
	ClientPending = "pending"
	ClientActive  = "active"
	ClientRevoked = "revoked"
)

// usableClientStatuses is the allowlist of oauth_client.status values that may
// start or continue an OAuth flow. Everything else — `revoked`, and any status
// a later migration invents — is refused, so this fails CLOSED by default
// rather than needing a new denial branch per new status.
//
// WHY `pending` IS ON THE LIST TODAY, and what has to change before it comes
// off: D-3 defines `pending` as "client minted, provisioning not yet
// confirmed", with oc-platform flipping it to `active` once the instance is
// up. That activation step does not exist yet — it belongs to sub-project #1.
// Meanwhile POST /api/oauth/self-hosted-client (RegisterSelfHosted, below)
// creates every client `pending`, and `oc dashboard register` expects the
// client_id it just received to work immediately. Rejecting `pending` today
// would therefore break the only self-hosted registration path there is, for
// every caller, with no way to recover — a strictly worse bug than the latent
// one this allowlist closes.
//
// This does NOT paint sub-project #1 into a corner: when the reconcile job and
// the instance-confirmation flip land, dropping ClientPending from this slice
// is a one-line change and every gate below tightens at once, because they all
// route through StatusUsable rather than each testing status themselves.
var usableClientStatuses = []string{ClientPending, ClientActive}

// ErrClientNotUsable is returned when a client exists but its status bars it
// from OAuth flows. Callers must NOT surface it as a distinct wire error: the
// device and token endpoints already collapse "unknown client" and "wrong
// client" into a single response code as an anti-probing measure, and giving
// a revoked client its own observable code would hand an attacker a free
// client_id oracle.
var ErrClientNotUsable = errors.New("oauth client is not usable")

// StatusUsable reports whether a client in this status may start or continue
// an OAuth flow. See usableClientStatuses for the policy and its expiry
// condition.
func StatusUsable(status string) bool {
	for _, s := range usableClientStatuses {
		if status == s {
			return true
		}
	}
	return false
}

// maxClientNameLen is the Postgres column's VARCHAR(64) limit
// (migrations/902_oauth_client.sql), counted the way Postgres counts it: in
// CHARACTERS, via utf8.RuneCountInString.
//
// It was previously checked with Go's len(), i.e. BYTES. That direction is
// safe against the database (byte length >= rune length, so passing implies
// the column accepts it) but it over-rejects: 25 emoji are 25 characters and
// 100 bytes, and any CJK name of ~22 characters or more was refused with a
// bare 400. Inferno explicitly serves CJK markets — Chinese/Japanese READMEs,
// WeChat/DingTalk/LinuxDo sign-in — so a byte limit on a user-visible display
// name is a real, user-facing defect for its primary users, not a nit. Runes
// match VARCHAR(64) exactly, so this is both more permissive and more precise.
const maxClientNameLen = 64

// ErrClientNameTooLong is returned by RegisterSelfHosted when the caller
// supplies a name longer than the schema allows. Handlers should map this
// to a 400, not a 500.
var ErrClientNameTooLong = errors.New("oauth client name exceeds maximum length")

// Docker-style name parts. Mirrors hermes_cli/dashboard_register.py so
// registered agents read the same on both surfaces.
var (
	nameAdjectives = []string{
		"amber", "bold", "brave", "bright", "calm", "clever", "cosmic", "crisp",
		"dreamy", "eager", "electric", "fancy", "gentle", "golden", "happy",
		"hidden", "jolly", "keen", "lively", "lucid", "lunar", "mellow", "merry",
		"mighty", "nimble", "noble", "polished", "quiet", "quirky", "rapid",
		"serene", "sharp", "shiny", "silent", "snappy", "solar", "spry", "stellar",
		"sunny", "swift", "tidy", "vivid", "vibrant", "witty", "zesty",
	}
	nameNouns = []string{
		"albatross", "antelope", "badger", "beacon", "comet", "condor", "cypress",
		"dolphin", "ember", "falcon", "ferret", "galaxy", "glacier", "harbor",
		"heron", "ibex", "jaguar", "kestrel", "lantern", "lynx", "meadow", "nebula",
		"ocelot", "orchid", "otter", "panther", "petrel", "quasar", "raven", "reef",
		"sparrow", "summit", "tundra", "vortex", "walrus", "willow", "yarrow",
		"kepler", "tesla", "curie", "hopper", "turing", "lovelace",
	}
)

// OAuthClientService owns the oauth_client registry: registration and
// lookup of PUBLIC OAuth 2.0 clients. There is deliberately no
// client_secret — PKCE is the protection.
type OAuthClientService struct {
	entClient *dbent.Client
}

func NewOAuthClientService(entClient *dbent.Client) *OAuthClientService {
	return &OAuthClientService{entClient: entClient}
}

func pick(list []string) string {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(list))))
	if err != nil {
		// Cosmetic-name entropy only — never the client_id. Falling back to
		// list[0] is safe, but silently always landing on the same word
		// under a persistent entropy failure is worth surfacing, not hiding.
		slog.Warn("oauth: name entropy read failed, using fallback word", "error", err)
		return list[0]
	}
	return list[n.Int64()]
}

// GenerateName returns a docker-style adjective_noun label. There is no
// uniqueness constraint — collisions are harmless, the row id is the key.
func (s *OAuthClientService) GenerateName() string {
	return pick(nameAdjectives) + "_" + pick(nameNouns)
}

// RegisterSelfHosted creates a SELF_HOSTED client owned by the given org.
// The "agent:" prefix is applied SERVER-side; callers never construct it —
// a client that could choose its own client_id could impersonate another
// agent.
//
// name is the caller-supplied label (e.g. `oc dashboard register --name`).
// Leading/trailing whitespace is trimmed; an empty or whitespace-only name
// falls back to GenerateName(). A non-empty name longer than the schema's
// MaxLen(64) is rejected with ErrClientNameTooLong rather than silently
// truncated — truncation would make the client refer to itself by a label
// it never chose, the same class of bug this parameter exists to fix. name
// is otherwise trusted verbatim: it is a user-chosen label, not something
// that needs a charset policy, and (like GenerateName's output) it is
// deliberately NOT required to be unique — the row id is the key.
func (s *OAuthClientService) RegisterSelfHosted(ctx context.Context, orgID, userID int64, redirectOrigin, name string) (*dbent.OAuthClient, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		name = s.GenerateName()
	case utf8.RuneCountInString(name) > maxClientNameLen:
		return nil, ErrClientNameTooLong
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("client id entropy: %w", err)
	}
	clientID := "agent:" + hex.EncodeToString(buf)

	created, err := s.entClient.OAuthClient.Create().
		SetClientID(clientID).
		SetKind("SELF_HOSTED").
		SetName(name).
		SetOwnerUserID(userID).
		SetOrgID(orgID).
		SetStatus(ClientPending).
		SetRedirectURIOrigin(redirectOrigin).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create oauth client: %w", err)
	}
	return created, nil
}

// ByClientID loads a client by its public client_id, WITHOUT any status
// filtering — it answers "does this row exist", nothing more. Anything gating
// an OAuth flow must use UsableByClientID instead.
func (s *OAuthClientService) ByClientID(ctx context.Context, clientID string) (*dbent.OAuthClient, error) {
	return s.entClient.OAuthClient.Query().
		Where(oauthclient.ClientID(clientID)).
		Only(ctx)
}

// UsableByClientID loads a client and refuses it unless its status permits
// OAuth flows, returning ErrClientNotUsable when it does not.
//
// This is the enforcement half of the `status` column. Until now nothing read
// it: neither the device authorization request nor either token grant
// consulted it, so a `revoked` client would have completed a full device flow
// and kept refreshing — the column advertised a kill switch that did not
// exist, while sub-project #1's reconcile job is being designed against it.
// The status is re-checked on every grant, not only at flow start, so revoking
// a client also stops an already-issued refresh family at its next rotation
// rather than only blocking new logins.
func (s *OAuthClientService) UsableByClientID(ctx context.Context, clientID string) (*dbent.OAuthClient, error) {
	client, err := s.ByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if !StatusUsable(client.Status) {
		return nil, ErrClientNotUsable
	}
	return client, nil
}
