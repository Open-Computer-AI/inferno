package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/org"
	"github.com/Wei-Shaw/sub2api/ent/orgmember"
)

// Org membership role constants. Deliberately named OrgRole* rather than
// Role* — service.RoleAdmin already exists (domain_constants.go, aliasing
// domain.RoleAdmin == "admin") for the platform-wide user role and is a
// different concept from an org-scoped role. Reusing RoleAdmin here would
// either fail to compile (duplicate const) or, worse, silently shadow a
// different value. The wire-format strings themselves are unchanged: these
// exact values ("OWNER"/"ADMIN"/"MEMBER") are read verbatim by the desktop
// client (apps/desktop/electron/main.ts) — do not change casing or values.
const (
	OrgRoleOwner  = "OWNER"
	OrgRoleAdmin  = "ADMIN"
	OrgRoleMember = "MEMBER"
)

// OrgService owns tenancy: personal-org creation and membership lookup.
type OrgService struct {
	entClient *dbent.Client
}

func NewOrgService(entClient *dbent.Client) *OrgService {
	return &OrgService{entClient: entClient}
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// slugFor derives a URL-safe slug from a username, with a random suffix so two
// users named "admin" cannot collide on the unique index.
func slugFor(username string) (string, error) {
	base := slugUnsafe.ReplaceAllString(strings.ToLower(strings.TrimSpace(username)), "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "org"
	}
	if len(base) > 48 {
		base = base[:48]
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("slug suffix: %w", err)
	}
	return base + "-" + hex.EncodeToString(buf), nil
}

// EnsurePersonalOrg returns the user's personal org, creating it on first call.
// Idempotent: safe to call on every login, and safe under concurrent callers
// for the same user (e.g. a double-fired OAuth callback, or two tabs finishing
// signup at once).
//
// Concurrency safety is enforced at the database level via a unique index on
// orgs.personal_user_id (nulls excluded — see ent/schema/org.go), not by
// application-level locking, because Inferno runs multiple server processes
// and an in-process lock would not be visible across them. The lookup below
// queries that same column directly (a single indexed read, and a clearer
// expression of the invariant than scanning org_members), and the create
// path below handles losing the race: if a concurrent caller's insert wins,
// this caller's insert fails the unique constraint, and we re-read and
// return the winner's row rather than propagating an error — the contract
// is "idempotent, returns the user's personal org," and the loser of the
// race must get a correct result too.
func (s *OrgService) EnsurePersonalOrg(ctx context.Context, userID int64, username string) (*dbent.Org, error) {
	existing, err := s.entClient.Org.Query().
		Where(org.PersonalUserID(userID)).
		Only(ctx)
	switch {
	case err == nil:
		return existing, nil
	case !dbent.IsNotFound(err):
		return nil, fmt.Errorf("query personal org: %w", err)
	}

	slug, err := slugFor(username)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(username)
	if name == "" {
		name = slug
	}

	created, err := s.entClient.Org.Create().
		SetSlug(slug).
		SetName(name).
		SetIsPersonal(true).
		SetPersonalUserID(userID).
		Save(ctx)
	if err != nil {
		if dbent.IsConstraintError(err) {
			// Lost the race: another caller's create for the same userID won
			// between our lookup and our insert. Re-read and return their row.
			winner, getErr := s.entClient.Org.Query().
				Where(org.PersonalUserID(userID)).
				Only(ctx)
			if getErr != nil {
				return nil, fmt.Errorf("re-read personal org after race: %w", getErr)
			}
			return winner, nil
		}
		return nil, fmt.Errorf("create personal org: %w", err)
	}

	if _, err := s.entClient.OrgMember.Create().
		SetOrgID(created.ID).
		SetUserID(userID).
		SetRole(OrgRoleOwner).
		Save(ctx); err != nil {
		return nil, fmt.Errorf("create owner membership: %w", err)
	}

	return created, nil
}

// OrgsForUser lists every org the user belongs to.
func (s *OrgService) OrgsForUser(ctx context.Context, userID int64) ([]*dbent.Org, error) {
	members, err := s.entClient.OrgMember.Query().
		Where(orgmember.UserID(userID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query memberships: %w", err)
	}
	orgs := make([]*dbent.Org, 0, len(members))
	for _, m := range members {
		org, err := s.entClient.Org.Get(ctx, m.OrgID)
		if err != nil {
			return nil, fmt.Errorf("load org %d: %w", m.OrgID, err)
		}
		orgs = append(orgs, org)
	}
	return orgs, nil
}

// RoleIn returns the user's role in an org, or "" when they are not a member.
func (s *OrgService) RoleIn(ctx context.Context, orgID, userID int64) (string, error) {
	m, err := s.entClient.OrgMember.Query().
		Where(orgmember.OrgID(orgID), orgmember.UserID(userID)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query membership: %w", err)
	}
	return m.Role, nil
}
