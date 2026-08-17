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
// Concurrency safety against creating TWO personal orgs is enforced at the
// database level via a unique index on orgs.personal_user_id (nulls excluded
// — see ent/schema/org.go), not by application-level locking, because Inferno
// runs multiple server processes and an in-process lock would not be visible
// across them. The lookup below queries that same column directly (a single
// indexed read, and a clearer expression of the invariant than scanning
// org_members).
//
// The org row and its OWNER membership row are written inside a single
// transaction so a crash between the two inserts can never leave a user with
// an org but no membership — that state would be a permanent lockout, not a
// self-healing one: a later EnsurePersonalOrg call short-circuits on finding
// the org by personal_user_id and would otherwise return immediately without
// ever creating the membership, while every membership-based lookup (like
// OrgsForUser, which callers use to pick the user's org) would keep coming up
// empty. As defense in depth against any row that already ended up in that
// state before this fix (or a first insert whose commit succeeded but whose
// caller never observed it), the short-circuit lookup also self-heals via
// ensureOwnerMembership below.
//
// The create path also handles losing the race: if a concurrent caller's
// transaction commits first, this caller's insert fails the unique
// constraint, the half-started transaction is rolled back, and we re-read
// and self-heal the winner's row rather than propagating an error — the
// contract is "idempotent, returns the user's personal org, with a working
// OWNER membership," and the loser of the race must get a correct result too.
func (s *OrgService) EnsurePersonalOrg(ctx context.Context, userID int64, username string) (*dbent.Org, error) {
	existing, err := s.entClient.Org.Query().
		Where(org.PersonalUserID(userID)).
		Only(ctx)
	switch {
	case err == nil:
		return s.ensureOwnerMembership(ctx, existing, userID)
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

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	created, err := tx.Org.Create().
		SetSlug(slug).
		SetName(name).
		SetIsPersonal(true).
		SetPersonalUserID(userID).
		Save(ctx)
	if err != nil {
		if dbent.IsConstraintError(err) {
			// Lost the race: another caller's transaction for the same userID
			// committed between our lookup and our insert. Roll back this
			// half-started transaction and re-read + self-heal the winner's row.
			_ = tx.Rollback()
			winner, getErr := s.entClient.Org.Query().
				Where(org.PersonalUserID(userID)).
				Only(ctx)
			if getErr != nil {
				return nil, fmt.Errorf("re-read personal org after race: %w", getErr)
			}
			return s.ensureOwnerMembership(ctx, winner, userID)
		}
		return nil, fmt.Errorf("create personal org: %w", err)
	}

	if _, err := tx.OrgMember.Create().
		SetOrgID(created.ID).
		SetUserID(userID).
		SetRole(OrgRoleOwner).
		Save(ctx); err != nil {
		return nil, fmt.Errorf("create owner membership: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit personal org creation: %w", err)
	}

	return created, nil
}

// ensureOwnerMembership guarantees an OWNER org_members row exists for userID
// in o, creating it if missing, and returns o unchanged either way. This
// repairs rows left in the broken "org exists, membership does not" state
// (see EnsurePersonalOrg's doc comment). Idempotent, and safe under a
// concurrent repairer: a unique-constraint violation on the create means
// another caller already fixed it — same "check-then-create, tolerate the
// race" idiom used elsewhere in this codebase (see
// internal/repository/simple_mode_default_groups.go's createGroupIfNotExists).
func (s *OrgService) ensureOwnerMembership(ctx context.Context, o *dbent.Org, userID int64) (*dbent.Org, error) {
	exists, err := s.entClient.OrgMember.Query().
		Where(orgmember.OrgID(o.ID), orgmember.UserID(userID)).
		Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("query owner membership: %w", err)
	}
	if exists {
		return o, nil
	}

	if _, err := s.entClient.OrgMember.Create().
		SetOrgID(o.ID).
		SetUserID(userID).
		SetRole(OrgRoleOwner).
		Save(ctx); err != nil && !dbent.IsConstraintError(err) {
		return nil, fmt.Errorf("repair owner membership: %w", err)
	}

	return o, nil
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
