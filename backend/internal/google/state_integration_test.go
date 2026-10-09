package google

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
)

func TestStateSingleUseExpiryAndOwner(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	legacySchema, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(legacySchema)); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	store := StateStore{Pool: pool, Now: func() time.Time { return now }}
	state, err := store.Create(ctx, "gsc", "owner-a", "/settings", "https://app.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(ctx, "gsc", state, "owner-b"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("wrong owner: %v", err)
	}
	if _, err := store.Consume(ctx, "gsc", state, "owner-a"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("replayed state: %v", err)
	}
	state, err = store.Create(ctx, "ga4", "owner-a", "/settings", "https://app.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(ctx, "gsc", state, "owner-a"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("wrong provider: %v", err)
	}
	result, err := store.Consume(ctx, "ga4", state, "owner-a")
	if err != nil || result.CallbackPath != "/settings" {
		t.Fatalf("valid state: %+v, %v", result, err)
	}
	state, err = store.Create(ctx, "gsc", "owner-a", "/settings", "https://app.example")
	if err != nil {
		t.Fatal(err)
	}
	store.Now = func() time.Time { return now.Add(stateTTL) }
	if _, err := store.Consume(ctx, "gsc", state, "owner-a"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expired state: %v", err)
	}
}
