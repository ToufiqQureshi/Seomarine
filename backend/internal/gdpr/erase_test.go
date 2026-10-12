package gdpr

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

func TestFindPlanAndExecuteEraseOneUsersData(t *testing.T) {
	ctx, pool, eraser := openEraser(t)
	id := rand.Text()
	userID, email := "gdpr-user-"+id, "gdpr-"+id+"@example.com"
	orgID, projectID := "gdpr-org-"+id, "gdpr-project-"+id
	seedSubject(ctx, t, pool, userID, email, orgID, projectID)
	t.Cleanup(func() { cleanupSubject(ctx, t, pool, orgID, userID) })
	if _, err := pool.Exec(ctx, `INSERT INTO session (id,token,user_id,expires_at,updated_at) VALUES ($1,$2,$3,now()+interval '1 hour',now())`, "session-"+id, "token-"+id, userID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO verification (id,identifier,value,expires_at) VALUES ($1,$2,$3,now()+interval '1 hour')`, "verification-"+id, email, userID); err != nil {
		t.Fatalf("insert verification token: %v", err)
	}

	subject, err := eraser.Find(ctx, strings.ToUpper(email), "")
	if err != nil || subject.ID != userID {
		t.Fatalf("Find() = %+v, %v; want user %q", subject, err, userID)
	}
	if _, err := eraser.Find(ctx, email, userID); err == nil {
		t.Fatal("Find() accepted both email and user ID")
	}
	if _, err := eraser.Find(ctx, "", "missing-user"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("Find(missing) error = %v, want ErrUserNotFound", err)
	}

	plan, err := eraser.Plan(ctx, subject)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Organizations) != 1 || plan.Organizations[0].ID != orgID || len(plan.ProjectIDs) != 1 || plan.ProjectIDs[0] != projectID {
		t.Fatalf("Plan() missed owned workspace/project: %+v", plan)
	}
	if plan.Rows[`session.user_id`] != 1 || plan.Rows[`member.user_id`] != 1 {
		t.Fatalf("Plan() reference counts = %v", plan.Rows)
	}
	if err := eraser.Execute(ctx, plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for table, query := range map[string]string{
		"user":         `SELECT count(*) FROM "user" WHERE id=$1`,
		"organization": `SELECT count(*) FROM organization WHERE id=$1`,
		"project":      `SELECT count(*) FROM projects WHERE id=$1`,
		"session":      `SELECT count(*) FROM session WHERE user_id=$1`,
		"verification": `SELECT count(*) FROM verification WHERE value=$1`,
	} {
		var remaining int
		if err := pool.QueryRow(ctx, query, map[string]string{"user": userID, "organization": orgID, "project": projectID, "session": userID, "verification": userID}[table]).Scan(&remaining); err != nil {
			t.Fatalf("count remaining %s rows: %v", table, err)
		}
		if remaining != 0 {
			t.Errorf("erasure left %d %s rows", remaining, table)
		}
	}
}

func TestPlanAndExecuteRefuseSharedOrganization(t *testing.T) {
	ctx, pool, eraser := openEraser(t)
	id := rand.Text()
	userID, email := "gdpr-shared-"+id, "gdpr-shared-"+id+"@example.com"
	otherID, otherEmail := "gdpr-other-"+id, "gdpr-other-"+id+"@example.com"
	orgID, projectID := "gdpr-shared-org-"+id, "gdpr-shared-project-"+id
	seedSubject(ctx, t, pool, userID, email, orgID, projectID)
	seedUser(ctx, t, pool, otherID, otherEmail)
	t.Cleanup(func() {
		cleanupSubject(ctx, t, pool, orgID, userID)
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM "user" WHERE id=$1`, otherID); err != nil {
			t.Errorf("delete other test user: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO member (id,organization_id,user_id,role,created_at) VALUES ($1,$2,$3,'member',now())`, "member-"+id, orgID, otherID); err != nil {
		t.Fatalf("add shared organization member: %v", err)
	}
	subject, err := eraser.Find(ctx, "", userID)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	_, err = eraser.Plan(ctx, subject)
	if !errors.Is(err, ErrSharedOrganization) {
		t.Fatalf("Plan() error = %v, want ErrSharedOrganization", err)
	}
	if err := eraser.Execute(ctx, Plan{Subject: subject, Organizations: []Organization{{ID: orgID}}, ProjectIDs: []string{projectID}}); !errors.Is(err, ErrSharedOrganization) {
		t.Fatalf("Execute() error = %v, want ErrSharedOrganization after dry-run race", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM "user" WHERE id=$1`, userID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("shared user's row count = %d, err = %v; want 1", remaining, err)
	}
}

func openEraser(t *testing.T) (context.Context, *pgxpool.Pool, Eraser) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	pool, err := pgdb.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return ctx, pool, Eraser{DB: pool}
}

func seedSubject(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID, email, orgID, projectID string) {
	t.Helper()
	seedUser(ctx, t, pool, userID, email)
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id,name,slug,created_at) VALUES ($1,'GDPR test',$1,now())`, orgID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO member (id,organization_id,user_id,role,created_at) VALUES ($1,$2,$3,'owner',now())`, "member-"+orgID, orgID, userID); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id,organization_id,name) VALUES ($1,$2,'GDPR test project')`, projectID, orgID); err != nil {
		t.Fatalf("insert project: %v", err)
	}
}

func seedUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID, email string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO "user" (id,name,email) VALUES ($1,'GDPR test',$2)`, userID, email); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func cleanupSubject(ctx context.Context, t *testing.T, pool *pgxpool.Pool, orgID, userID string) {
	t.Helper()
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(cleanupCtx, `DELETE FROM organization WHERE id=$1`, orgID); err != nil {
		t.Errorf("delete test organization: %v", err)
	}
	if _, err := pool.Exec(cleanupCtx, `DELETE FROM "user" WHERE id=$1`, userID); err != nil {
		t.Errorf("delete test user: %v", err)
	}
}
