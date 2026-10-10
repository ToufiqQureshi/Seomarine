package projectcontext

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

// Repository owns SQL for shared project memory. Project-level updates hold a
// row lock so concurrently validated batches cannot exceed storage caps.
type Repository struct{ DB *pgxpool.Pool }

func (r Repository) Get(ctx context.Context, projectID string) (ProjectContext, error) {
	if r.DB == nil {
		return ProjectContext{}, fmt.Errorf("project context database unavailable")
	}
	return Load(ctx, r.DB, projectID)
}

func (r Repository) Apply(ctx context.Context, projectID, author string, now time.Time, resolveBatch func(ProjectContext) ([]resolvedOp, error)) (ProjectContext, error) {
	if r.DB == nil {
		return ProjectContext{}, fmt.Errorf("project context database unavailable")
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ProjectContext{}, fmt.Errorf("begin project context update: %w", err)
	}
	defer tx.Rollback(ctx)
	var locked string
	err = tx.QueryRow(ctx, `SELECT id FROM projects WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, projectID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectContext{}, &Error{Code: "NOT_FOUND", Message: "Project not found."}
	}
	if err != nil {
		return ProjectContext{}, fmt.Errorf("lock project context: %w", err)
	}
	current, err := Load(ctx, tx, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	ops, err := resolveBatch(current)
	if err != nil {
		return ProjectContext{}, err
	}
	stamp := now.UTC().Format("2006-01-02T15:04:05.000Z")
	for _, op := range ops {
		if err := applyOperation(ctx, tx, projectID, author, stamp, now.UTC(), op); err != nil {
			return ProjectContext{}, fmt.Errorf("apply project context %s: %w", op.kind, err)
		}
	}
	result, err := Load(ctx, tx, projectID)
	if err != nil {
		return ProjectContext{}, fmt.Errorf("read updated project context: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ProjectContext{}, fmt.Errorf("commit project context update: %w", err)
	}
	return result, nil
}

func applyOperation(ctx context.Context, tx pgx.Tx, project, author, stamp string, now time.Time, op resolvedOp) error {
	switch op.kind {
	case "delete_section":
		_, err := tx.Exec(ctx, `DELETE FROM project_context_sections WHERE project_id=$1 AND key=$2`, project, op.key)
		return err
	case "upsert_section":
		_, err := tx.Exec(ctx, `INSERT INTO project_context_sections(project_id,key,title,content,updated_at,updated_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(project_id,key) DO UPDATE SET title=COALESCE(EXCLUDED.title,project_context_sections.title),content=EXCLUDED.content,updated_at=EXCLUDED.updated_at,updated_by=EXCLUDED.updated_by`, project, op.key, nullString(op.title), op.content, stamp, author)
		return err
	case "upsert_competitors":
		for _, v := range op.competitors {
			id, err := ids.New()
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO project_competitors(id,project_id,domain,name,notes,updated_at,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(project_id,domain) DO UPDATE SET name=COALESCE(EXCLUDED.name,project_competitors.name),notes=COALESCE(EXCLUDED.notes,project_competitors.notes),updated_at=EXCLUDED.updated_at,updated_by=EXCLUDED.updated_by`, id, project, v.domain, v.name, v.notes, stamp, author); err != nil {
				return err
			}
		}
		return nil
	case "delete_competitors":
		_, err := tx.Exec(ctx, `DELETE FROM project_competitors WHERE project_id=$1 AND domain=ANY($2::text[])`, project, op.domains)
		return err
	case "upsert_pages":
		for _, v := range op.pages {
			id, err := ids.New()
			if err != nil {
				return err
			}
			role := v.role
			if role == "" {
				role = "other"
			}
			_, err = tx.Exec(ctx, `INSERT INTO project_key_pages(id,project_id,url,role,topic,notes,updated_at,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(project_id,url) DO UPDATE SET role=CASE WHEN $9 THEN EXCLUDED.role ELSE project_key_pages.role END,topic=COALESCE(EXCLUDED.topic,project_key_pages.topic),notes=COALESCE(EXCLUDED.notes,project_key_pages.notes),updated_at=EXCLUDED.updated_at,updated_by=EXCLUDED.updated_by`, id, project, v.url, role, v.topic, v.notes, stamp, author, v.role != "")
			if err != nil {
				return err
			}
		}
		return nil
	case "delete_pages":
		_, err := tx.Exec(ctx, `DELETE FROM project_key_pages WHERE project_id=$1 AND url=ANY($2::text[])`, project, op.urls)
		return err
	case "delete_research":
		_, err := tx.Exec(ctx, `DELETE FROM project_research_log WHERE project_id=$1 AND id=ANY($2::text[])`, project, op.ids)
		return err
	case "append_research":
		id, err := ids.New()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO project_research_log(id,project_id,entry_date,summary,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6)`, id, project, now.Format("2006-01-02"), op.summary, author, stamp)
		if err != nil {
			return err
		}
		cutoff := now.AddDate(0, 0, -researchLogRetentionDays).Format("2006-01-02")
		_, err = tx.Exec(ctx, `DELETE FROM project_research_log WHERE project_id=$1 AND entry_date<$2`, project, cutoff)
		return err
	default:
		return fmt.Errorf("unknown project context operation %q", op.kind)
	}
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
