package onboarding

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

func TestRepositorySaveKeepsGSCNudgeSeparateFromCompletion(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	pool, err := pgdb.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	id := "onboarding-test-" + rand.Text()
	userID, organizationID := "user-"+id, "org-"+id
	if _, err := pool.Exec(ctx, `INSERT INTO "user" (id,name,email) VALUES ($1,'Test user',$2)`, userID, id+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id,name,slug,created_at) VALUES ($1,'Test org',$1,now())`, organizationID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM organization WHERE id=$1`, organizationID); err != nil {
			t.Errorf("delete test organization: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM "user" WHERE id=$1`, userID); err != nil {
			t.Errorf("delete test user: %v", err)
		}
	})

	repo := Repository{DB: pool}
	interests := []string{"analytics", "rank tracking"}
	if err := repo.Save(ctx, userID, organizationID, SaveInput{
		InterestedFeatures: &interests,
		Completed:          true,
	}); err != nil {
		t.Fatalf("save onboarding answers: %v", err)
	}
	answers, err := repo.Get(ctx, userID)
	if err != nil {
		t.Fatalf("get onboarding answers: %v", err)
	}
	if answers.CompletedAt == nil {
		t.Fatal("completed onboarding has no completion timestamp")
	}
	if answers.GSCNudgeDismissedAt != nil {
		t.Fatalf("saving completed answers also dismissed GSC nudge at %q", *answers.GSCNudgeDismissedAt)
	}
	if len(answers.Answers.InterestedFeatures) != 2 || answers.Answers.InterestedFeatures[0] != interests[0] {
		t.Fatalf("saved interests = %v, want %v", answers.Answers.InterestedFeatures, interests)
	}

	if err := repo.DismissGSCNudge(ctx, userID, organizationID); err != nil {
		t.Fatalf("dismiss GSC nudge: %v", err)
	}
	answers, err = repo.Get(ctx, userID)
	if err != nil {
		t.Fatalf("get dismissed nudge: %v", err)
	}
	if answers.GSCNudgeDismissedAt == nil {
		t.Fatal("dismissed GSC nudge has no timestamp")
	}
	dismissedAt := *answers.GSCNudgeDismissedAt

	workFor := "agency"
	if err := repo.Save(ctx, userID, organizationID, SaveInput{WorkFor: &workFor, Completed: true}); err != nil {
		t.Fatalf("update onboarding answers: %v", err)
	}
	answers, err = repo.Get(ctx, userID)
	if err != nil {
		t.Fatalf("get updated answers: %v", err)
	}
	if answers.GSCNudgeDismissedAt == nil || *answers.GSCNudgeDismissedAt != dismissedAt {
		t.Fatalf("updating answers changed GSC dismissal from %q to %v", dismissedAt, answers.GSCNudgeDismissedAt)
	}
	if answers.Answers.WorkFor == nil || *answers.Answers.WorkFor != workFor {
		t.Fatalf("saved workFor = %v, want %q", answers.Answers.WorkFor, workFor)
	}
}
