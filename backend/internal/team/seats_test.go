package team

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	pool, err := pgdb.Open(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	schema, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	return pool
}

func TestInvitationsStopAtThePlanMemberLimit(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t)
	org, owner := "org-"+rand.Text(), "user-"+rand.Text()
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Seats', $1, now())`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO "user" (id, name, email, email_verified) VALUES ($1, 'Owner', $2, true)`, owner, owner+"@uplumo.test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean := context.WithoutCancel(ctx)
		if _, err := pool.Exec(clean, `DELETE FROM organization WHERE id = $1`, org); err != nil {
			t.Errorf("clean organization: %v", err)
		}
		if _, err := pool.Exec(clean, `DELETE FROM "user" WHERE id = $1`, owner); err != nil {
			t.Errorf("clean user: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO member (id, organization_id, user_id, role, created_at) VALUES ($1, $2, $1, 'owner', now())`, owner, org); err != nil {
		t.Fatal(err)
	}

	s := &Service{DB: pool}
	user := auth.User{ID: owner, OrganizationID: org, Role: "owner"}
	now := time.Now().UTC()
	invite := func(email string, seats int) error {
		_, _, err := s.saveInvitation(ctx, user, email, now, seats)
		return err
	}

	// One seat is the owner's own: nobody can be invited.
	if err := invite("first@uplumo.test", 1); !errors.Is(err, ErrSeatLimit) {
		t.Fatalf("invite into a 1-seat plan error = %v, want ErrSeatLimit", err)
	}
	// Three seats: the owner and two invitations, which count before they are accepted.
	for _, email := range []string{"first@uplumo.test", "second@uplumo.test"} {
		if err := invite(email, 3); err != nil {
			t.Fatalf("invite %s into a 3-seat plan error = %v", email, err)
		}
	}
	if err := invite("third@uplumo.test", 3); !errors.Is(err, ErrSeatLimit) {
		t.Fatalf("fourth person in a 3-seat plan error = %v, want ErrSeatLimit", err)
	}
	// Sending an invitation again takes no new seat.
	if err := invite("FIRST@uplumo.test", 3); err != nil {
		t.Errorf("re-inviting a pending person at the limit error = %v, want it refreshed", err)
	}
	// No limit: staff workspaces and servers without plans.
	if err := invite("third@uplumo.test", 0); err != nil {
		t.Errorf("invite without a limit error = %v", err)
	}
}

type fakeMemberLimits struct {
	limit int
	err   error
}

func (f fakeMemberLimits) MemberLimit(context.Context, string) (int, error) { return f.limit, f.err }

func TestInviteFailsWhenThePlanCannotBeRead(t *testing.T) {
	failure := errors.New("billing down")
	s := &Service{Limits: fakeMemberLimits{err: failure}}
	_, err := s.Invite(context.Background(), auth.User{ID: "user_1", OrganizationID: "org-" + rand.Text(), Role: "owner"}, "new@uplumo.test")
	if !errors.Is(err, failure) {
		t.Fatalf("Invite() error = %v, want the plan failure passed on", err)
	}
}
