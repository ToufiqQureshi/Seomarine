// Package contenteditor stores project-scoped SEO and AEO writing drafts.
package contenteditor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/ids"
)

const (
	// MaxTitleLength is the maximum allowed draft title length.
	MaxTitleLength = 180
	// MaxKeywordLength is the maximum allowed target keyword length.
	MaxKeywordLength = 120
	// MaxSlugLength is the maximum allowed draft URL slug length.
	MaxSlugLength = 300
	// MaxMetaLength is the maximum allowed meta description length.
	MaxMetaLength = 320
	// MaxContentLength is the maximum allowed draft content size in bytes.
	MaxContentLength = 150_000
	// MaxDraftsPerProject is the maximum number of drafts allowed per project.
	MaxDraftsPerProject = 100
	// DraftListLimit is the number of drafts returned by one list request.
	DraftListLimit = 100
	// MaxIDLength bounds the author id a client may send.
	MaxIDLength = 128
	// MaxDateLength bounds the review date string a client may send.
	MaxDateLength = 40
)

// querier is the subset of pgxpool.Pool the author check needs.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ErrNotFound indicates that a requested content draft does not exist.
var ErrNotFound = errors.New("content draft not found")

// ErrDraftLimit indicates that the project has reached its draft limit.
var ErrDraftLimit = errors.New("project has reached the content draft limit")

// ValidationError describes invalid content draft input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// Draft is a saved content draft.
type Draft struct {
	ID              string `json:"id"`
	ProjectID       string `json:"projectId"`
	Title           string `json:"title"`
	TargetKeyword   string `json:"targetKeyword"`
	Slug            string `json:"slug"`
	MetaDescription string `json:"metaDescription"`
	Content         string `json:"content"`
	AuthorID        string `json:"authorId,omitempty"`
	// LastReviewedAt is the UTC midnight of the review date, or nil.
	LastReviewedAt *time.Time `json:"lastReviewedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// DraftSummary is the list view of a saved content draft.
type DraftSummary struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"projectId"`
	Title         string    `json:"title"`
	TargetKeyword string    `json:"targetKeyword"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Input contains the values used to create or update a draft.
type Input struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	TargetKeyword   string `json:"targetKeyword"`
	Slug            string `json:"slug"`
	MetaDescription string `json:"metaDescription"`
	Content         string `json:"content"`
	// AuthorID selects a go_content_authors row of the same project; empty
	// means no author.
	AuthorID string `json:"authorId,omitempty"`
	// LastReviewedDate is the review date as YYYY-MM-DD; empty clears it.
	LastReviewedDate string `json:"lastReviewedDate,omitempty"`
}

// Repository stores and retrieves content drafts.
type Repository struct{ DB *pgxpool.Pool }

// List returns the drafts for a project.
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

// Get returns a project draft by ID.
func (r Repository) Get(ctx context.Context, projectID, id string) (Draft, error) {
	draft, err := scanDraft(r.DB.QueryRow(ctx, `SELECT id, project_id, title, target_keyword, slug, meta_description, content, author_id, last_reviewed_at, created_at, updated_at
		FROM go_content_drafts WHERE project_id=$1 AND id=$2`, projectID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	if err != nil {
		return Draft{}, fmt.Errorf("get content draft: %w", err)
	}
	return draft, nil
}

// Save creates or updates a project draft.
func (r Repository) Save(ctx context.Context, projectID string, input Input) (result Draft, retErr error) {
	if err := validate(input); err != nil {
		return Draft{}, err
	}
	reviewed, err := reviewedDate(input.LastReviewedDate)
	if err != nil {
		return Draft{}, err
	}
	if input.ID != "" {
		if input.AuthorID != "" {
			if err := r.checkAuthor(ctx, r.DB, projectID, input.AuthorID); err != nil {
				return Draft{}, err
			}
		}
		updated, err := scanDraft(r.DB.QueryRow(ctx, `UPDATE go_content_drafts SET title=$3, target_keyword=$4, slug=$5,
			meta_description=$6, content=$7, author_id=$8, last_reviewed_at=$9, updated_at=now()
			WHERE id=$1 AND project_id=$2
			RETURNING id, project_id, title, target_keyword, slug, meta_description, content, author_id, last_reviewed_at, created_at, updated_at`,
			input.ID, projectID, strings.TrimSpace(input.Title), strings.TrimSpace(input.TargetKeyword), strings.TrimSpace(input.Slug), strings.TrimSpace(input.MetaDescription), input.Content,
			authorRef(input.AuthorID), reviewed))
		if !errors.Is(err, pgx.ErrNoRows) {
			if err != nil {
				if author, missing := missingAuthor(err); missing {
					return Draft{}, &ValidationError{Message: fmt.Sprintf("The author %s is no longer available on this project. Pick another author.", author)}
				}
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
	defer func(rollbackCtx context.Context) {
		if err := tx.Rollback(rollbackCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("rollback create content draft: %w", err))
		}
	}(ctx)
	var lockedProjectID string
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, projectID).Scan(&lockedProjectID); errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, ErrNotFound
	} else if err != nil {
		return Draft{}, fmt.Errorf("lock content draft project: %w", err)
	}
	if input.AuthorID != "" {
		if err := r.checkAuthor(ctx, tx, projectID, input.AuthorID); err != nil {
			return Draft{}, err
		}
	}
	var existing int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM go_content_drafts WHERE project_id=$1`, projectID).Scan(&existing); err != nil {
		return Draft{}, fmt.Errorf("count project content drafts: %w", err)
	}
	if existing >= MaxDraftsPerProject {
		return Draft{}, ErrDraftLimit
	}
	draft, err := scanDraft(tx.QueryRow(ctx, `INSERT INTO go_content_drafts
		(id, project_id, title, target_keyword, slug, meta_description, content, author_id, last_reviewed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, project_id, title, target_keyword, slug, meta_description, content, author_id, last_reviewed_at, created_at, updated_at`,
		id, projectID, strings.TrimSpace(input.Title), strings.TrimSpace(input.TargetKeyword), strings.TrimSpace(input.Slug), strings.TrimSpace(input.MetaDescription), input.Content,
		authorRef(input.AuthorID), reviewed))
	if err != nil {
		return Draft{}, fmt.Errorf("create content draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Draft{}, fmt.Errorf("commit content draft: %w", err)
	}
	return draft, nil
}

// Delete removes a project draft.
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
	if len(in.AuthorID) > MaxIDLength {
		return &ValidationError{Message: "the author selection is not valid"}
	}
	return nil
}

func scanDraft(row pgx.Row) (Draft, error) {
	var draft Draft
	var authorID *string
	err := row.Scan(&draft.ID, &draft.ProjectID, &draft.Title, &draft.TargetKeyword, &draft.Slug,
		&draft.MetaDescription, &draft.Content, &authorID, &draft.LastReviewedAt,
		&draft.CreatedAt, &draft.UpdatedAt)
	if err != nil {
		return Draft{}, err
	}
	if authorID != nil {
		draft.AuthorID = *authorID
	}
	return draft, nil
}

// authorRef maps an empty author selection to a database NULL.
func authorRef(authorID string) any {
	if authorID == "" {
		return nil
	}
	return authorID
}

// reviewedDate parses the review date the writer's browser sent. The editor
// only ever sends "YYYY-MM-DD"; anything else is refused instead of
// silently moved to another day.
func reviewedDate(input string) (*time.Time, error) {
	if input == "" {
		return nil, nil
	}
	if len(input) > MaxDateLength {
		return nil, &ValidationError{Message: "the review date is not a valid date"}
	}
	parsed, err := time.Parse("2006-01-02", input)
	if err != nil || parsed.Format("2006-01-02") != input {
		return nil, &ValidationError{Message: "the review date is not a valid date"}
	}
	day := parsed.UTC()
	return &day, nil
}

// checkAuthor refuses any author that is not on this project, so a draft can
// never be signed by another workspace's author.
func (r Repository) checkAuthor(ctx context.Context, querier querier, projectID, authorID string) error {
	var found string
	err := querier.QueryRow(ctx, `SELECT id FROM go_content_authors WHERE id=$1 AND project_id=$2`, authorID, projectID).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return &ValidationError{Message: "The author must be picked from this project's author profiles."}
	}
	if err != nil {
		return fmt.Errorf("check content author: %w", err)
	}
	return nil
}

// missingAuthor reports whether err is the (rare) deletion race: the author
// was removed between the check and the write, and the foreign key caught it.
func missingAuthor(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" || !strings.Contains(pgErr.Detail, "go_content_authors") {
		return "", false
	}
	return "selection", true
}
