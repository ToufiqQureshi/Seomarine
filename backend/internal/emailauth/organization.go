package emailauth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// activeOrganization is the workspace a sign-in lands in.
type activeOrganization struct {
	ID   string
	Role string
}

// resolveSignInOrganization picks the organization a new session starts in:
// the last one the user worked in while they still belong to it, else the one
// they joined most recently (so a fresh invite beats the personal workspace),
// else a new default workspace. A user with no workspace and a pending
// invitation gets none: someone who signs up from an invite link should end up
// in exactly the inviter's organization, not that plus an empty personal one.
// The next app request mints a workspace if they abandon the invite.
func resolveSignInOrganization(ctx context.Context, q querier, user User, now time.Time) (*activeOrganization, error) {
	existing, err := existingOrganization(ctx, q, user.ID)
	if err != nil || existing != nil {
		return existing, err
	}
	pending, err := hasPendingInvitation(ctx, q, user.Email, now)
	if err != nil || pending {
		return nil, err
	}
	return createDefaultOrganization(ctx, q, user, now)
}

func existingOrganization(ctx context.Context, q querier, userID string) (*activeOrganization, error) {
	var last *string
	if err := q.QueryRow(ctx, `SELECT last_active_organization_id FROM "user" WHERE id = $1`, userID).Scan(&last); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("read last active organization: %w", err)
	}
	if last != nil {
		var role string
		err := q.QueryRow(ctx, `SELECT role FROM member WHERE user_id = $1 AND organization_id = $2 LIMIT 1`, userID, *last).Scan(&role)
		if err == nil {
			return &activeOrganization{ID: *last, Role: role}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("check last active membership: %w", err)
		}
	}
	var org activeOrganization
	err := q.QueryRow(ctx, `SELECT organization_id, role FROM member WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID).Scan(&org.ID, &org.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find newest membership: %w", err)
	}
	return &org, nil
}

func hasPendingInvitation(ctx context.Context, q querier, email string, now time.Time) (bool, error) {
	var one int
	err := q.QueryRow(ctx, `SELECT 1 FROM invitation
		WHERE lower(email) = $1 AND status = 'pending' AND expires_at > $2 LIMIT 1`,
		strings.ToLower(strings.TrimSpace(email)), now).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check pending invitations: %w", err)
	}
	return true, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-"), "-")
	if len(slug) > 48 {
		slug = slug[:48]
	}
	if slug == "" {
		return "organization"
	}
	return slug
}

func hexOf(value string) string { return fmt.Sprintf("%x", []byte(value)) }

// createDefaultOrganization gives a user their one personal workspace, with the
// user as its owner. It is deterministic in the user id, so two sign-ins racing
// each other collide on the slug and the loser adopts the winner's workspace
// instead of creating a second one.
func createDefaultOrganization(ctx context.Context, q querier, user User, now time.Time) (*activeOrganization, error) {
	label := strings.TrimSpace(user.Name)
	if label == "" {
		label, _, _ = strings.Cut(user.Email, "@")
	}
	if label == "" {
		label = "Uplumo"
	}
	slugSource := label
	if slugSource == "" {
		slugSource = "organization"
	}
	suffix := hexOf(user.ID)
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	slug := slugify(slugSource) + "-" + suffix

	orgID, err := newID(32)
	if err != nil {
		return nil, err
	}
	memberID, err := newID(32)
	if err != nil {
		return nil, err
	}
	// A savepoint keeps a lost race from poisoning the surrounding transaction.
	tx, err := beginNested(ctx, q)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, $2, $3, $4)`,
		orgID, label+"'s organization", slug, now); err != nil {
		_ = tx.Rollback(ctx)
		if isUniqueViolation(err) {
			return adoptExistingOrganization(ctx, q, user.ID)
		}
		return nil, fmt.Errorf("create organization: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO member (id, organization_id, user_id, role, created_at) VALUES ($1, $2, $3, 'owner', $4)`,
		memberID, orgID, user.ID, now); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("create owner membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit organization: %w", err)
	}
	return &activeOrganization{ID: orgID, Role: "owner"}, nil
}

// adoptExistingOrganization is the fallback for a lost creation race: use
// whatever workspace the user already has.
func adoptExistingOrganization(ctx context.Context, q querier, userID string) (*activeOrganization, error) {
	var org activeOrganization
	err := q.QueryRow(ctx, `SELECT organization_id, role FROM member WHERE user_id = $1 ORDER BY created_at ASC LIMIT 1`, userID).Scan(&org.ID, &org.Role)
	if err != nil {
		return nil, fmt.Errorf("adopt existing organization: %w", err)
	}
	return &org, nil
}

// beginNested starts a savepoint when q is a transaction.
func beginNested(ctx context.Context, q querier) (pgx.Tx, error) {
	tx, ok := q.(pgx.Tx)
	if !ok {
		return nil, errors.New("organization bootstrap must run inside a transaction")
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin savepoint: %w", err)
	}
	return nested, nil
}
