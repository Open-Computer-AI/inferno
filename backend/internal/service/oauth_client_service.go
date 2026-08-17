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

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthclient"
)

const (
	ClientPending = "pending"
	ClientActive  = "active"
	ClientRevoked = "revoked"
)

// maxClientNameLen mirrors the ent schema's field.String("name").MaxLen(64)
// (backend/ent/schema/oauth_client.go). Checked here so an over-length name
// gets a clean 400 instead of an opaque database error.
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
	case len(name) > maxClientNameLen:
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

// ByClientID loads a client by its public client_id.
func (s *OAuthClientService) ByClientID(ctx context.Context, clientID string) (*dbent.OAuthClient, error) {
	return s.entClient.OAuthClient.Query().
		Where(oauthclient.ClientID(clientID)).
		Only(ctx)
}
