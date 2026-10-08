package pgdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := testPool(ctx, t)
	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("query: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", one)
	}
}

func TestOpenRejectsMalformedAndUnreachableURLs(t *testing.T) {
	if pool, err := Open(context.Background(), "postgres://%zz"); err == nil {
		pool.Close()
		t.Fatal("Open accepted malformed URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if pool, err := Open(ctx, "postgres://seomarine@127.0.0.1:1/seomarine?connect_timeout=2"); err == nil {
		pool.Close()
		t.Fatal("Open connected to a closed local port")
	}
}

func TestInTxCommitsAndRollsBack(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := testPool(ctx, t)
	committedTable := testTableName(t)
	if err := InTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "CREATE TABLE "+committedTable+" (id integer PRIMARY KEY)"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO "+committedTable+" (id) VALUES (1)")
		return err
	}); err != nil {
		t.Fatalf("InTx commit: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DROP TABLE IF EXISTS "+committedTable); err != nil {
			t.Errorf("drop committed table: %v", err)
		}
	})
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+committedTable).Scan(&count); err != nil {
		t.Fatalf("read committed table: %v", err)
	}
	if count != 1 {
		t.Fatalf("committed rows = %d, want 1", count)
	}

	rolledBackTable := testTableName(t)
	callbackErr := errors.New("abort transaction")
	err := InTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "CREATE TABLE "+rolledBackTable+" (id integer)"); err != nil {
			return err
		}
		return callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("InTx error = %v, want callback error", err)
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+rolledBackTable).Scan(&exists); err != nil {
		t.Fatalf("check rolled back table: %v", err)
	}
	if exists {
		t.Fatalf("table %q exists after callback error", rolledBackTable)
	}
}

func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testTableName(t *testing.T) string {
	t.Helper()
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("go_pgdb_tx_%s", hex.EncodeToString(suffix[:]))
}
