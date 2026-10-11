// Package gdpr erases a user's account and the data that belongs only to them.
//
// Everything lives in one Postgres database, so erasure is one transaction. The
// foreign keys cascade from the user and from each workspace, and a sweep over
// every table that carries a user, organization or project id removes whatever
// the cascade did not reach, including tables added after this was written.
package gdpr

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSharedOrganization means a workspace the user belongs to has other members.
// Erasing it would destroy their data too, so a human must settle it first.
var ErrSharedOrganization = errors.New("user belongs to a shared organization")

// ErrUserNotFound means no user matched the selector.
var ErrUserNotFound = errors.New("no matching user")

// Subject is the person to erase.
type Subject struct {
	ID    string
	Email string
	Name  string
}

// Organization is a workspace the subject owns alone.
type Organization struct {
	ID   string
	Name string
}

// Plan is what an erasure would remove.
type Plan struct {
	Subject       Subject
	Organizations []Organization
	ProjectIDs    []string
	// Rows counts, per "table.column", the rows that reference the erased ids.
	Rows map[string]int64
}

// Eraser plans and runs erasures.
type Eraser struct{ DB *pgxpool.Pool }

type column struct{ table, name string }

func (c column) key() string { return c.table + "." + c.name }

// Find resolves a selector: an email (any case) or a user id.
func (e Eraser) Find(ctx context.Context, email, userID string) (Subject, error) {
	if (email == "") == (userID == "") {
		return Subject{}, errors.New("pass exactly one of email or user id")
	}
	var s Subject
	var err error
	if email != "" {
		err = e.DB.QueryRow(ctx, `SELECT id, email, name FROM "user" WHERE lower(email) = lower($1)`, email).Scan(&s.ID, &s.Email, &s.Name)
	} else {
		err = e.DB.QueryRow(ctx, `SELECT id, email, name FROM "user" WHERE id = $1`, userID).Scan(&s.ID, &s.Email, &s.Name)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Subject{}, ErrUserNotFound
	}
	if err != nil {
		return Subject{}, fmt.Errorf("find user: %w", err)
	}
	return s, nil
}

// referenceColumns lists every base table column that holds one of the ids.
func referenceColumns(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) ([]column, error) {
	rows, err := q.Query(ctx, `SELECT c.table_name, c.column_name
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = current_schema() AND t.table_type = 'BASE TABLE'
			AND c.column_name IN ('user_id', 'organization_id', 'project_id')
		ORDER BY c.table_name, c.column_name`)
	if err != nil {
		return nil, fmt.Errorf("list reference columns: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (column, error) {
		var c column
		return c, row.Scan(&c.table, &c.name)
	})
}

func idsFor(c column, userID string, orgIDs, projectIDs []string) []string {
	switch c.name {
	case "user_id":
		return []string{userID}
	case "organization_id":
		return orgIDs
	default:
		return projectIDs
	}
}

// Plan inventories an erasure without changing anything.
func (e Eraser) Plan(ctx context.Context, subject Subject) (Plan, error) {
	plan := Plan{Subject: subject, Rows: map[string]int64{}}
	rows, err := e.DB.Query(ctx, `SELECT o.id, o.name, (SELECT count(*) FROM member x WHERE x.organization_id = o.id)
		FROM member m JOIN organization o ON o.id = m.organization_id WHERE m.user_id = $1 ORDER BY o.id`, subject.ID)
	if err != nil {
		return Plan{}, fmt.Errorf("list organizations: %w", err)
	}
	type orgRow struct {
		Organization
		members int
	}
	orgs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (orgRow, error) {
		var o orgRow
		return o, row.Scan(&o.ID, &o.Name, &o.members)
	})
	if err != nil {
		return Plan{}, fmt.Errorf("read organizations: %w", err)
	}
	var shared []string
	for _, o := range orgs {
		if o.members != 1 {
			shared = append(shared, fmt.Sprintf("%s (%d members)", o.ID, o.members))
		}
		plan.Organizations = append(plan.Organizations, o.Organization)
	}
	if len(shared) > 0 {
		return Plan{}, fmt.Errorf("%w: %s. Transfer or remove the user first", ErrSharedOrganization, strings.Join(shared, ", "))
	}

	orgIDs := make([]string, len(plan.Organizations))
	for i, o := range plan.Organizations {
		orgIDs[i] = o.ID
	}
	projectRows, err := e.DB.Query(ctx, `SELECT id FROM projects WHERE organization_id = ANY($1) ORDER BY id`, orgIDs)
	if err != nil {
		return Plan{}, fmt.Errorf("list projects: %w", err)
	}
	if plan.ProjectIDs, err = pgx.CollectRows(projectRows, pgx.RowTo[string]); err != nil {
		return Plan{}, fmt.Errorf("read projects: %w", err)
	}

	columns, err := referenceColumns(ctx, e.DB)
	if err != nil {
		return Plan{}, err
	}
	for _, c := range columns {
		var n int64
		query := fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = ANY($1)`, pgx.Identifier{c.table}.Sanitize(), pgx.Identifier{c.name}.Sanitize())
		if err := e.DB.QueryRow(ctx, query, idsFor(c, subject.ID, orgIDs, plan.ProjectIDs)).Scan(&n); err != nil {
			return Plan{}, fmt.Errorf("count %s: %w", c.key(), err)
		}
		if n > 0 {
			plan.Rows[c.key()] = n
		}
	}
	return plan, nil
}

// Execute erases the subject in one transaction. The plan is recomputed inside
// the transaction's view of the data by the caller having just produced it; a
// workspace that gained a member in between makes the sweep delete only what
// still references the erased ids, never the new member's account.
func (e Eraser) Execute(ctx context.Context, plan Plan) error {
	orgIDs := make([]string, len(plan.Organizations))
	for i, o := range plan.Organizations {
		orgIDs[i] = o.ID
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin erasure: %w", err)
	}
	defer tx.Rollback(ctx)

	// Re-check under lock that nobody joined a workspace since the dry run.
	var others int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM member WHERE organization_id = ANY($1) AND user_id <> $2`, orgIDs, plan.Subject.ID).Scan(&others); err != nil {
		return fmt.Errorf("recheck members: %w", err)
	}
	if others > 0 {
		return fmt.Errorf("%w: a member joined after the dry run", ErrSharedOrganization)
	}

	steps := []struct {
		what, sql string
		args      []any
	}{
		{"reset tokens", `DELETE FROM verification WHERE value = $1`, []any{plan.Subject.ID}},
		{"organizations", `DELETE FROM organization WHERE id = ANY($1)`, []any{orgIDs}},
		{"user", `DELETE FROM "user" WHERE id = $1`, []any{plan.Subject.ID}},
	}
	for _, step := range steps {
		if _, err := tx.Exec(ctx, step.sql, step.args...); err != nil {
			return fmt.Errorf("erase %s: %w", step.what, err)
		}
	}

	// The cascade is the main path; this sweep catches tables without a foreign
	// key and any added later.
	columns, err := referenceColumns(ctx, tx)
	if err != nil {
		return err
	}
	// Tables that others reference go last, so a child row never blocks its parent.
	slices.SortStableFunc(columns, func(a, b column) int { return strings.Compare(b.table, a.table) })
	for _, c := range columns {
		query := fmt.Sprintf(`DELETE FROM %s WHERE %s = ANY($1)`, pgx.Identifier{c.table}.Sanitize(), pgx.Identifier{c.name}.Sanitize())
		if _, err := tx.Exec(ctx, query, idsFor(c, plan.Subject.ID, orgIDs, plan.ProjectIDs)); err != nil {
			return fmt.Errorf("sweep %s: %w", c.key(), err)
		}
	}

	var remaining int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM "user" WHERE id = $1`, plan.Subject.ID).Scan(&remaining); err != nil || remaining != 0 {
		return errors.New("user row still present after erasure")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit erasure: %w", err)
	}
	return nil
}
