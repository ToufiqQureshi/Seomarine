package sam

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openSamTestDB returns a pool on a private schema holding the tables SAM reads
// plus the migration under test, so tests cannot see each other's rows.
func openSamTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("sam_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(stop, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	legacy := `
CREATE TABLE "user" (id text PRIMARY KEY, email text NOT NULL);
CREATE TABLE member (id text PRIMARY KEY, organization_id text NOT NULL, user_id text NOT NULL, role text NOT NULL);
CREATE TABLE projects (id text PRIMARY KEY, organization_id text NOT NULL, name text NOT NULL, domain text,
	location_code integer NOT NULL DEFAULT 2840, language_code text NOT NULL DEFAULT 'en', archived_at text);
CREATE TABLE sam_sessions (id text PRIMARY KEY, project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	user_id text NOT NULL REFERENCES "user"(id) ON DELETE CASCADE, title text NOT NULL DEFAULT 'New chat',
	created_at text NOT NULL DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	updated_at text NOT NULL DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), archived_at text);`
	if _, err := pool.Exec(ctx, legacy); err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}
	migration, err := os.ReadFile("../database/migrations/00014_sam_messages.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, _, _ := strings.Cut(string(migration), "-- +goose Down")
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	seedSamRows(ctx, t, pool)
	return pool
}

func seedSamRows(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	statements := []string{
		`INSERT INTO "user" VALUES ('user-1', 'owner@example.test'), ('user-2', 'other@example.test')`,
		`INSERT INTO member VALUES ('m1', 'org-1', 'user-1', 'admin')`,
		`INSERT INTO projects (id, organization_id, name, domain) VALUES ('project-1', 'org-1', 'Acme', 'acme.test'), ('project-2', 'org-1', 'Other', NULL)`,
		`INSERT INTO sam_sessions (id, project_id, user_id) VALUES ('session-1', 'project-1', 'user-1'), ('session-2', 'project-1', 'user-1')`,
	}
	for _, s := range statements {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
}

func TestMessageRepositoryOrdersAndRoundTripsParts(t *testing.T) {
	pool := openSamTestDB(t)
	repo := MessageRepository{DB: pool}
	ctx := context.Background()

	if msgs, err := repo.List(ctx, "session-1"); err != nil || msgs == nil || len(msgs) != 0 {
		t.Fatalf("empty session = %v err = %v (must be a non-nil empty slice)", msgs, err)
	}
	assistant := Message{ID: "a1", Role: "assistant", Parts: []Part{
		{Type: "step-start"},
		{Type: "tool-get_domain_overview", ToolCallID: "c1", State: stateOutputAvailable,
			Input: []byte(`{"domain":"é.test"}`), Output: []byte(`{"summary":"ok","data":{"rows":[1,2]}}`)},
		{Type: "text", Text: "unicode ✓ with a NUL \x00 inside", State: stateDone},
	}}
	for _, m := range []Message{
		{ID: "u1", Role: "user", Parts: []Part{{Type: "text", Text: "hi"}}},
		assistant,
		{ID: "u2", Role: "user", Parts: []Part{{Type: "text", Text: "again"}}},
	} {
		if err := repo.Append(ctx, "session-1", m); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Append(ctx, "session-2", Message{ID: "other", Role: "user", Parts: []Part{{Type: "text", Text: "x"}}}); err != nil {
		t.Fatal(err)
	}

	msgs, err := repo.List(ctx, "session-1")
	if err != nil || len(msgs) != 3 {
		t.Fatalf("messages = %+v err = %v", msgs, err)
	}
	for i, id := range []string{"u1", "a1", "u2"} {
		if msgs[i].ID != id {
			t.Errorf("position %d = %s, want %s", i, msgs[i].ID, id)
		}
	}
	if msgs[0].Seq >= msgs[1].Seq || msgs[1].Seq >= msgs[2].Seq {
		t.Errorf("seq must increase: %d %d %d", msgs[0].Seq, msgs[1].Seq, msgs[2].Seq)
	}
	tool := msgs[1].Parts[1]
	if got := msgs[1].Parts[2].Text; got != "unicode ✓ with a NUL  inside" {
		t.Errorf("text = %q (NUL must be stripped, everything else kept)", got)
	}
	if tool.State != stateOutputAvailable || string(tool.Input) != `{"domain": "é.test"}` && string(tool.Input) != `{"domain":"é.test"}` {
		t.Errorf("tool part did not round-trip: %+v", tool)
	}
	if !strings.Contains(string(tool.Output), `"rows": [1, 2]`) && !strings.Contains(string(tool.Output), `"rows":[1,2]`) {
		t.Errorf("tool output = %s", tool.Output)
	}
}

func TestMessageRepositoryRejectsDuplicateIDsPerSession(t *testing.T) {
	pool := openSamTestDB(t)
	repo := MessageRepository{DB: pool}
	ctx := context.Background()
	m := Message{ID: "dup", Role: "user", Parts: []Part{{Type: "text", Text: "x"}}}
	if err := repo.Append(ctx, "session-1", m); err != nil {
		t.Fatal(err)
	}
	err := repo.Append(ctx, "session-1", m)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate error = %v; the handler maps 23505 to 409", err)
	}
	if err := repo.Append(ctx, "session-2", m); err != nil {
		t.Fatalf("the same id in another session is fine: %v", err)
	}
	bad := Message{ID: "bad", Role: "system", Parts: nil}
	if err := repo.Append(ctx, "session-1", bad); err == nil {
		t.Fatal("only user and assistant roles may be stored")
	}
}

func TestMessageRepositoryDeleteFromIsScopedAndDropsStaleSummary(t *testing.T) {
	pool := openSamTestDB(t)
	repo := MessageRepository{DB: pool}
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c", "d"} {
		if err := repo.Append(ctx, "session-1", Message{ID: id, Role: "user", Parts: []Part{{Type: "text", Text: id}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Append(ctx, "session-2", Message{ID: "b", Role: "user", Parts: []Part{{Type: "text", Text: "keep"}}}); err != nil {
		t.Fatal(err)
	}
	all, _ := repo.List(ctx, "session-1")
	if err := repo.SaveSummary(ctx, "session-1", Summary{Text: "covers a..c", ThroughSeq: all[2].Seq}); err != nil {
		t.Fatal(err)
	}

	found, err := repo.DeleteFrom(ctx, "session-1", "c")
	if err != nil || !found {
		t.Fatalf("found = %v err = %v", found, err)
	}
	msgs, _ := repo.List(ctx, "session-1")
	if len(msgs) != 2 || msgs[1].ID != "b" {
		t.Fatalf("remaining = %+v", msgs)
	}
	if other, _ := repo.List(ctx, "session-2"); len(other) != 1 {
		t.Fatalf("another session's identically named message was deleted: %+v", other)
	}
	if _, ok, _ := repo.Summary(ctx, "session-1"); ok {
		t.Fatal("a summary of deleted messages must not survive")
	}

	if found, err := repo.DeleteFrom(ctx, "session-1", "missing"); err != nil || found {
		t.Fatalf("missing message: found = %v err = %v", found, err)
	}

	// A summary entirely before the rewind point stays valid.
	_ = repo.Append(ctx, "session-1", Message{ID: "e", Role: "user", Parts: []Part{{Type: "text", Text: "e"}}})
	msgs, _ = repo.List(ctx, "session-1")
	_ = repo.SaveSummary(ctx, "session-1", Summary{Text: "covers a", ThroughSeq: msgs[0].Seq})
	if _, err := repo.DeleteFrom(ctx, "session-1", "e"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := repo.Summary(ctx, "session-1"); !ok {
		t.Fatal("a still-accurate summary must be kept")
	}
}

func TestSummaryUpsertAndCascade(t *testing.T) {
	pool := openSamTestDB(t)
	repo := MessageRepository{DB: pool}
	ctx := context.Background()
	if _, ok, err := repo.Summary(ctx, "session-1"); ok || err != nil {
		t.Fatalf("no summary yet: ok = %v err = %v", ok, err)
	}
	_ = repo.SaveSummary(ctx, "session-1", Summary{Text: "one", ThroughSeq: 3})
	_ = repo.SaveSummary(ctx, "session-1", Summary{Text: "two", ThroughSeq: 9})
	got, ok, _ := repo.Summary(ctx, "session-1")
	if !ok || got.Text != "two" || got.ThroughSeq != 9 {
		t.Fatalf("summary = %+v", got)
	}
	_ = repo.Append(ctx, "session-1", Message{ID: "x", Role: "user", Parts: []Part{{Type: "text", Text: "x"}}})
	if _, err := pool.Exec(ctx, `DELETE FROM sam_sessions WHERE id='session-1'`); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM go_sam_messages) + (SELECT count(*) FROM go_sam_session_summaries)`).Scan(&n)
	if n != 0 {
		t.Fatalf("deleting a session left %d transcript rows", n)
	}
}

func TestContextLoaderScopesToTheOwnersActiveSession(t *testing.T) {
	pool := openSamTestDB(t)
	loader := ContextLoader{DB: pool}
	ctx := context.Background()

	sc, err := loader.Load(ctx, "project-1", "user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if sc.UserEmail != "owner@example.test" || sc.OrganizationID != "org-1" || sc.Role != "admin" ||
		sc.Project.Name != "Acme" || sc.Project.Domain != "acme.test" || sc.Project.LocationCode != 2840 || sc.Title != "New chat" {
		t.Fatalf("context = %+v", sc)
	}

	for name, args := range map[string][3]string{
		"wrong user":    {"project-1", "user-2", "session-1"},
		"wrong project": {"project-2", "user-1", "session-1"},
		"no session":    {"project-1", "user-1", "nope"},
	} {
		if _, err := loader.Load(ctx, args[0], args[1], args[2]); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("%s: err = %v, want ErrSessionNotFound", name, err)
		}
	}

	if _, err := pool.Exec(ctx, `UPDATE sam_sessions SET archived_at='2026-01-01' WHERE id='session-2'`); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(ctx, "project-1", "user-1", "session-2"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("archived session: err = %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE projects SET archived_at='2026-01-01' WHERE id='project-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(ctx, "project-1", "user-1", "session-1"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("archived project: err = %v", err)
	}
}

func TestContextLoaderGivesARemovedMemberNoRole(t *testing.T) {
	pool := openSamTestDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM member`); err != nil {
		t.Fatal(err)
	}
	sc, err := ContextLoader{DB: pool}.Load(ctx, "project-1", "user-1", "session-1")
	if err != nil || sc.Role != "" {
		t.Fatalf("a removed member has no role: role = %q err = %v", sc.Role, err)
	}
}

func TestSetTitleAndTouchKeepTheLegacyTimestampFormat(t *testing.T) {
	pool := openSamTestDB(t)
	repo := Repository{DB: pool}
	ctx := context.Background()
	if err := repo.SetTitle(ctx, "session-1", "Quick wins"); err != nil {
		t.Fatal(err)
	}
	var title, updated string
	_ = pool.QueryRow(ctx, `SELECT title, updated_at FROM sam_sessions WHERE id='session-1'`).Scan(&title, &updated)
	if title != "Quick wins" {
		t.Fatalf("title = %q", title)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", updated); err != nil {
		t.Fatalf("updated_at %q is not in the format the registry list sorts and the client parses: %v", updated, err)
	}
	if err := repo.Touch(ctx, "session-2"); err != nil {
		t.Fatal(err)
	}
	var s1, s2 string
	_ = pool.QueryRow(ctx, `SELECT updated_at FROM sam_sessions WHERE id='session-2'`).Scan(&s2)
	_ = pool.QueryRow(ctx, `SELECT updated_at FROM sam_sessions WHERE id='session-1'`).Scan(&s1)
	if s2 < s1 {
		t.Fatalf("touch must move a session to the top: %s < %s", s2, s1)
	}
}
