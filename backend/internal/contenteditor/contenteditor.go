// Package contenteditor stores project-scoped SEO and AEO writing drafts.
package contenteditor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/ids"
)

const (
	MaxTitleLength      = 180
	MaxKeywordLength    = 120
	MaxSlugLength       = 300
	MaxMetaLength       = 320
	MaxContentLength    = 150_000
	MaxDraftsPerProject = 100
	DraftListLimit      = 100
)

var ErrNotFound = errors.New("content draft not found")
var ErrDraftLimit = errors.New("project has reached the content draft limit")

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type Draft struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"projectId"`
	Title           string    `json:"title"`
	TargetKeyword   string    `json:"targetKeyword"`
	Slug            string    `json:"slug"`
	MetaDescription string    `json:"metaDescription"`
	Content         string    `json:"content"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type DraftSummary struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"projectId"`
	Title         string    `json:"title"`
	TargetKeyword string    `json:"targetKeyword"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Input struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	TargetKeyword   string `json:"targetKeyword"`
	Slug            string `json:"slug"`
	MetaDescription string `json:"metaDescription"`
	Content         string `json:"content"`
}

type Repository struct{ DB *pgxpool.Pool }

func (r Repository) List(ctx context.Context, projectID string) ([]DraftSummary, error) {
	rows, err := r.DB.Query(ctx, `SELECT id, project_id, title, target_keyword, updated_at
		FROM go_content_drafts WHERE project_id=$1 ORDER BY updated_at DESC, id LIMIT $2`, projectID, DraftListLimit)
	if err != nil {
		return nil, fmt.Errorf("list content drafts: %w", err)
	}
	drafts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (DraftSummary, error) {
		var draft DraftSummary
		err := row.Scan(&draft.ID, &draft.ProjectID, &draft.Title, &draft.TargetKeyword, &draft.UpdatedAt)
		return draft, err
	})
	if err != nil {
		return nil, fmt.Errorf("read content drafts: %w", err)
	}
	if drafts == nil {
		drafts = []DraftSummary{}
	}
	return drafts, nil
}

func (r Repository) Get(ctx context.Context, projectID, id string) (Draft, error) {
	draft, err := scanDraft(r.DB.QueryRow(ctx, `SELECT id, project_id, title, target_keyword, slug, meta_description, content, created_at, updated_at
		FROM go_content_drafts WHERE project_id=$1 AND id=$2`, projectID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	if err != nil {
		return Draft{}, fmt.Errorf("get content draft: %w", err)
	}
	return draft, nil
}

func (r Repository) Save(ctx context.Context, projectID string, input Input) (Draft, error) {
	if err := validate(input); err != nil {
		return Draft{}, err
	}
	if input.ID != "" {
		updated, err := scanDraft(r.DB.QueryRow(ctx, `UPDATE go_content_drafts SET title=$3, target_keyword=$4, slug=$5,
			meta_description=$6, content=$7, updated_at=now() WHERE id=$1 AND project_id=$2
			RETURNING id, project_id, title, target_keyword, slug, meta_description, content, created_at, updated_at`,
			input.ID, projectID, strings.TrimSpace(input.Title), strings.TrimSpace(input.TargetKeyword), strings.TrimSpace(input.Slug), strings.TrimSpace(input.MetaDescription), input.Content))
		if !errors.Is(err, pgx.ErrNoRows) {
			if err != nil {
				return Draft{}, fmt.Errorf("update content draft: %w", err)
			}
			return updated, nil
		}
		return Draft{}, ErrNotFound
	}

	id, err := ids.New()
	if err != nil {
		return Draft{}, fmt.Errorf("create content draft id: %w", err)
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return Draft{}, fmt.Errorf("begin create content draft: %w", err)
	}
	defer tx.Rollback(ctx)
	var lockedProjectID string
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, projectID).Scan(&lockedProjectID); errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, ErrNotFound
	} else if err != nil {
		return Draft{}, fmt.Errorf("lock content draft project: %w", err)
	}
	var existing int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM go_content_drafts WHERE project_id=$1`, projectID).Scan(&existing); err != nil {
		return Draft{}, fmt.Errorf("count project content drafts: %w", err)
	}
	if existing >= MaxDraftsPerProject {
		return Draft{}, ErrDraftLimit
	}
	draft, err := scanDraft(tx.QueryRow(ctx, `INSERT INTO go_content_drafts
		(id, project_id, title, target_keyword, slug, meta_description, content)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, project_id, title, target_keyword, slug, meta_description, content, created_at, updated_at`,
		id, projectID, strings.TrimSpace(input.Title), strings.TrimSpace(input.TargetKeyword), strings.TrimSpace(input.Slug), strings.TrimSpace(input.MetaDescription), input.Content))
	if err != nil {
		return Draft{}, fmt.Errorf("create content draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Draft{}, fmt.Errorf("commit content draft: %w", err)
	}
	return draft, nil
}

func (r Repository) Delete(ctx context.Context, projectID, id string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM go_content_drafts WHERE project_id=$1 AND id=$2`, projectID, id)
	if err != nil {
		return fmt.Errorf("delete content draft: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func validate(in Input) error {
	if len([]rune(strings.TrimSpace(in.Title))) > MaxTitleLength {
		return &ValidationError{Message: fmt.Sprintf("title must be at most %d characters", MaxTitleLength)}
	}
	if len([]rune(strings.TrimSpace(in.TargetKeyword))) > MaxKeywordLength {
		return &ValidationError{Message: fmt.Sprintf("target keyword must be at most %d characters", MaxKeywordLength)}
	}
	if len([]rune(strings.TrimSpace(in.Slug))) > MaxSlugLength {
		return &ValidationError{Message: fmt.Sprintf("slug must be at most %d characters", MaxSlugLength)}
	}
	if len([]rune(strings.TrimSpace(in.MetaDescription))) > MaxMetaLength {
		return &ValidationError{Message: fmt.Sprintf("meta description must be at most %d characters", MaxMetaLength)}
	}
	if len(in.Content) > MaxContentLength {
		return &ValidationError{Message: fmt.Sprintf("content must be at most %d bytes", MaxContentLength)}
	}
	return nil
}

func scanDraft(row pgx.Row) (Draft, error) {
	var draft Draft
	err := row.Scan(&draft.ID, &draft.ProjectID, &draft.Title, &draft.TargetKeyword, &draft.Slug,
		&draft.MetaDescription, &draft.Content, &draft.CreatedAt, &draft.UpdatedAt)
	return draft, err
}
