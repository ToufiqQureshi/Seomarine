// Package articletracking stores opt-in monitoring for published content.
// Provider execution is deliberately injected by the application scheduler;
// page reads in this package only return stored results.
package articletracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("article tracking not found")
	ErrNotPublished = errors.New("draft has not been successfully published")
	ErrInvalidInput = errors.New("invalid article tracking input")
)

type Tracking struct {
	DraftID           string          `json:"draftId"`
	ProjectID         string          `json:"projectId"`
	PublishedURL      string          `json:"publishedUrl"`
	TargetKeyword     string          `json:"targetKeyword"`
	RankConfigID      *string         `json:"rankConfigId,omitempty"`
	AIPrompt          string          `json:"aiPrompt"`
	AINextCheckAt     time.Time       `json:"aiNextCheckAt"`
	AILastCheckedAt   *time.Time      `json:"aiLastCheckedAt,omitempty"`
	LatestAIResult    json.RawMessage `json:"latestAiResult,omitempty"`
	LatestAIStatus    string          `json:"latestAiStatus,omitempty"`
	LatestAIFetchedAt *time.Time      `json:"latestAiFetchedAt,omitempty"`
}

type OptInInput struct {
	DraftID        string
	ProjectID      string
	OrganizationID string
	TargetKeyword  string
}

type DueAI struct {
	DraftID        string
	ProjectID      string
	OrganizationID string
	PublishedURL   string
	TargetKeyword  string
	Prompt         string
}

// Repository persists the user opt-in and durable AI result snapshots.
type Repository interface {
	OptIn(context.Context, OptInInput) (Tracking, error)
	Get(context.Context, string, string, string) (Tracking, error)
	SetRankConfig(context.Context, string, string, string, string) error
	DueAI(context.Context, time.Time, int) ([]DueAI, error)
	SaveAIResult(context.Context, string, string, string, string, json.RawMessage, bool, time.Time, time.Time) error
}

type Store struct{ DB *pgxpool.Pool }

func (s Store) OptIn(ctx context.Context, in OptInInput) (Tracking, error) {
	if strings.TrimSpace(in.DraftID) == "" || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.OrganizationID) == "" || len([]rune(strings.TrimSpace(in.TargetKeyword))) > 120 || strings.TrimSpace(in.TargetKeyword) == "" {
		return Tracking{}, ErrInvalidInput
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Tracking{}, fmt.Errorf("begin article tracking opt-in: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var publishedURL string
	err = tx.QueryRow(ctx, `SELECT pub.remote_url FROM go_content_drafts d
		JOIN projects p ON p.id=d.project_id AND p.organization_id=$3 AND p.archived_at IS NULL
		JOIN go_site_publications pub ON pub.draft_id=d.id AND pub.project_id=d.project_id AND pub.remote_status='publish'
		WHERE d.id=$1 AND d.project_id=$2 FOR SHARE OF pub`, in.DraftID, in.ProjectID, in.OrganizationID).Scan(&publishedURL)
	if errors.Is(err, pgx.ErrNoRows) {
		var published bool
		checkErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM go_site_publications WHERE draft_id=$1 AND project_id=$2 AND remote_status='publish')`, in.DraftID, in.ProjectID).Scan(&published)
		if checkErr != nil {
			return Tracking{}, fmt.Errorf("check article publication: %w", checkErr)
		}
		if !published {
			return Tracking{}, ErrNotPublished
		}
		return Tracking{}, ErrNotFound
	}
	if err != nil {
		return Tracking{}, fmt.Errorf("load article publication: %w", err)
	}
	publishedURL, err = NormalizePublishedURL(publishedURL)
	if err != nil {
		return Tracking{}, err
	}
	var t Tracking
	err = tx.QueryRow(ctx, `INSERT INTO go_content_article_tracking
		(draft_id, project_id, organization_id, published_url, target_keyword, ai_prompt)
		SELECT d.id, d.project_id, p.organization_id, $5, $4,
			'What are the best resources for ' || $4 || '?'
		FROM go_content_drafts d
	JOIN projects p ON p.id=d.project_id AND p.organization_id=$3 AND p.archived_at IS NULL
	JOIN go_site_publications pub ON pub.draft_id=d.id AND pub.project_id=d.project_id AND pub.remote_status='publish'
	WHERE d.id=$1 AND d.project_id=$2
	ON CONFLICT (draft_id) DO UPDATE SET updated_at=go_content_article_tracking.updated_at
	WHERE go_content_article_tracking.project_id=EXCLUDED.project_id
	  AND go_content_article_tracking.organization_id=EXCLUDED.organization_id
	RETURNING draft_id, project_id, published_url, target_keyword, rank_config_id, ai_prompt, ai_next_check_at, ai_last_checked_at`,
		in.DraftID, in.ProjectID, in.OrganizationID, strings.TrimSpace(in.TargetKeyword), publishedURL).Scan(
		&t.DraftID, &t.ProjectID, &t.PublishedURL, &t.TargetKeyword, &t.RankConfigID, &t.AIPrompt, &t.AINextCheckAt, &t.AILastCheckedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tracking{}, ErrNotFound
	}
	if err != nil {
		return Tracking{}, fmt.Errorf("opt in article tracking: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Tracking{}, fmt.Errorf("commit article tracking opt-in: %w", err)
	}
	return t, nil
}

func (s Store) Get(ctx context.Context, orgID, projectID, draftID string) (Tracking, error) {
	var t Tracking
	var result []byte
	err := s.DB.QueryRow(ctx, `SELECT t.draft_id,t.project_id,t.published_url,t.target_keyword,t.rank_config_id,t.ai_prompt,
		t.ai_next_check_at,t.ai_last_checked_at,r.result,r.status,r.fetched_at
	FROM go_content_article_tracking t
	LEFT JOIN LATERAL (SELECT result,status,fetched_at FROM go_content_article_ai_results WHERE draft_id=t.draft_id ORDER BY fetched_at DESC,id DESC LIMIT 1) r ON true
	WHERE t.organization_id=$1 AND t.project_id=$2 AND t.draft_id=$3`, orgID, projectID, draftID).Scan(
		&t.DraftID, &t.ProjectID, &t.PublishedURL, &t.TargetKeyword, &t.RankConfigID, &t.AIPrompt, &t.AINextCheckAt, &t.AILastCheckedAt, &result, &t.LatestAIStatus, &t.LatestAIFetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tracking{}, ErrNotFound
	}
	if err != nil {
		return Tracking{}, fmt.Errorf("get article tracking: %w", err)
	}
	if len(result) > 0 {
		t.LatestAIResult = json.RawMessage(result)
	}
	return t, nil
}

func (s Store) SetRankConfig(ctx context.Context, orgID, projectID, draftID, configID string) error {
	tag, err := s.DB.Exec(ctx, `UPDATE go_content_article_tracking t SET rank_config_id=$4,updated_at=now()
	WHERE t.organization_id=$1 AND t.project_id=$2 AND t.draft_id=$3
	AND EXISTS (SELECT 1 FROM go_rank_tracking_configs c JOIN projects p ON p.id=c.project_id
		WHERE c.id=$4 AND c.project_id=t.project_id AND p.organization_id=t.organization_id)`, orgID, projectID, draftID, configID)
	if err != nil {
		return fmt.Errorf("set article rank tracker: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s Store) DueAI(ctx context.Context, now time.Time, limit int) ([]DueAI, error) {
	if limit < 1 || limit > 500 {
		return nil, ErrInvalidInput
	}
	rows, err := s.DB.Query(ctx, `SELECT draft_id,project_id,organization_id,published_url,target_keyword,ai_prompt
	FROM go_content_article_tracking WHERE ai_enabled AND ai_next_check_at <= $1 ORDER BY ai_next_check_at,draft_id LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due article AI tracking: %w", err)
	}
	defer rows.Close()
	out := make([]DueAI, 0)
	for rows.Next() {
		var d DueAI
		if err := rows.Scan(&d.DraftID, &d.ProjectID, &d.OrganizationID, &d.PublishedURL, &d.TargetKeyword, &d.Prompt); err != nil {
			return nil, fmt.Errorf("read due article AI tracking: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due article AI tracking: %w", err)
	}
	return out, nil
}

// SaveAIResult stores a completed provider outcome then advances its schedule.
// On provider errors callers persist a safe status payload, never error bodies.
func (s Store) SaveAIResult(ctx context.Context, orgID, projectID, draftID, model string, result json.RawMessage, success bool, fetchedAt, next time.Time) error {
	if strings.TrimSpace(model) == "" || len(result) > 1<<20 || !json.Valid(result) || next.Before(fetchedAt) {
		return ErrInvalidInput
	}
	status := "failed"
	if success {
		status = "success"
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin article AI result: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	err = tx.QueryRow(ctx, `SELECT true FROM go_content_article_tracking WHERE organization_id=$1 AND project_id=$2 AND draft_id=$3 FOR UPDATE`, orgID, projectID, draftID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock article AI tracking: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO go_content_article_ai_results(draft_id,model,status,result,fetched_at) VALUES($1,$2,$3,$4,$5)`, draftID, strings.TrimSpace(model), status, result, fetchedAt); err != nil {
		return fmt.Errorf("save article AI result: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE go_content_article_tracking SET ai_last_checked_at=$4,ai_next_check_at=$5,updated_at=now() WHERE organization_id=$1 AND project_id=$2 AND draft_id=$3`, orgID, projectID, draftID, fetchedAt, next); err != nil {
		return fmt.Errorf("advance article AI schedule: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit article AI result: %w", err)
	}
	return nil
}

func NormalizePublishedURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return "", ErrInvalidInput
	}
	u.Fragment = ""
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}
