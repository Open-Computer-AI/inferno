package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthclient"
)

const (
	ClientPending = "pending"
	ClientActive  = "active"
	ClientRevoked = "revoked"
)

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
func (s *OAuthClientService) RegisterSelfHosted(ctx context.Context, orgID, userID int64, redirectOrigin string) (*dbent.OAuthClient, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("client id entropy: %w", err)
	}
	clientID := "agent:" + hex.EncodeToString(buf)

	created, err := s.entClient.OAuthClient.Create().
		SetClientID(clientID).
		SetKind("SELF_HOSTED").
		SetName(s.GenerateName()).
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
