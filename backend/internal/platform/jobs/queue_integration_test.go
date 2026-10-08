package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

func TestQueueEnqueueClaimConcurrencyAndStaleClaims(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := jobsTestPool(ctx, t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	queue, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	name := "test-" + randomSuffix(t)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM go_jobs WHERE queue IN ($1, $2)", name, name+"-retry"); err != nil {
			t.Errorf("clean test jobs: %v", err)
		}
	})

	first, err := queue.Enqueue(ctx, EnqueueInput{Queue: name, IdempotencyKey: "same", Payload: json.RawMessage(`{"value":1}`)})
	if err != nil {
		t.Fatalf("enqueue first job: %v", err)
	}
	duplicate, err := queue.Enqueue(ctx, EnqueueInput{Queue: name, IdempotencyKey: "same", Payload: json.RawMessage(`{"value":2}`)})
	if err != nil {
		t.Fatalf("enqueue duplicate: %v", err)
	}
	if duplicate.ID != first.ID || string(duplicate.Payload) != string(first.Payload) {
		t.Fatalf("duplicate enqueue = %#v, want original job %#v", duplicate, first)
	}

	const total, workers = 40, 8
	for i := 0; i < total; i++ {
		if _, err := queue.Enqueue(ctx, EnqueueInput{Queue: name, Payload: json.RawMessage(`{}`)}); err != nil {
			t.Fatalf("enqueue job %d: %v", i, err)
		}
	}
	claimedIDs := make(chan int64, total+workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Go(func() {
			claimed, err := queue.Claim(ctx, name, "worker-"+randomSuffix(t), time.Minute, total)
			if err != nil {
				t.Errorf("claim jobs: %v", err)
				return
			}
			for _, job := range claimed {
				claimedIDs <- job.ID
			}
		})
	}
	wg.Wait()
	close(claimedIDs)
	seen := make(map[int64]bool, total+1)
	for id := range claimedIDs {
		if seen[id] {
			t.Fatalf("job %d was claimed more than once", id)
		}
		seen[id] = true
	}
	if len(seen) != total+1 {
		t.Fatalf("claimed %d distinct jobs, want %d", len(seen), total+1)
	}

	retryQueue := name + "-retry"
	if _, err := queue.Enqueue(ctx, EnqueueInput{Queue: retryQueue, Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("enqueue retry candidate: %v", err)
	}
	claimed, err := queue.Claim(ctx, retryQueue, "retry-worker", time.Second, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim retry candidate = %d jobs, err %v", len(claimed), err)
	}
	job := claimed[0]
	if err := queue.Retry(ctx, job, context.DeadlineExceeded); err != nil {
		t.Fatalf("retry job: %v", err)
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT state FROM go_jobs WHERE id = $1", job.ID).Scan(&state); err != nil {
		t.Fatalf("read retried job state: %v", err)
	}
	if state != "queued" {
		t.Fatalf("retried job state = %q, want queued", state)
	}
	if err := queue.Complete(ctx, job); err != ErrClaimLost {
		t.Fatalf("stale completion error = %v, want ErrClaimLost", err)
	}
	time.Sleep(1100 * time.Millisecond)
	reclaimed, err := queue.Claim(ctx, retryQueue, "replacement-worker", time.Second, 1)
	if err != nil || len(reclaimed) != 1 {
		t.Fatalf("reclaim retried job = %d jobs, err %v", len(reclaimed), err)
	}
	if reclaimed[0].ClaimVersion <= job.ClaimVersion {
		t.Fatalf("reclaim version = %d, want greater than %d", reclaimed[0].ClaimVersion, job.ClaimVersion)
	}
	if err := queue.Complete(ctx, reclaimed[0]); err != nil {
		t.Fatalf("complete reclaimed job: %v", err)
	}
}

func jobsTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	pool, err := pgdb.Open(ctx, url)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value[:])
}
