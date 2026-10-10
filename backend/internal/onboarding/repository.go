package onboarding

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ DB *pgxpool.Pool }

func (r Repository) Get(ctx context.Context, userID string) (Answers, error) {
	var created time.Time
	if err := r.DB.QueryRow(ctx, `SELECT created_at FROM "user" WHERE id=$1`, userID).Scan(&created); err != nil {
		return Answers{}, fmt.Errorf("load onboarding user: %w", err)
	}
	out := Answers{UserCreatedAt: stringPointer(created.UTC().Format(time.RFC3339Nano)), Answers: Saved{InterestedFeatures: []string{}}}
	var raw string
	err := r.DB.QueryRow(ctx, `SELECT completed_at,gsc_nudge_dismissed_at,interested_features,work_for,client_website_count,found_via
		FROM user_onboarding_answers WHERE user_id=$1`, userID).Scan(&out.CompletedAt, &out.GSCNudgeDismissedAt, &raw,
		&out.Answers.WorkFor, &out.Answers.ClientWebsiteCount, &out.Answers.FoundVia)
	if err == pgx.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return Answers{}, fmt.Errorf("load onboarding answers: %w", err)
	}
	var values []string
	if json.Unmarshal([]byte(raw), &values) == nil && values != nil {
		out.Answers.InterestedFeatures = values
	}
	return out, nil
}

func (r Repository) Save(ctx context.Context, userID, organizationID string, in SaveInput) error {
	features, err := marshalFeatures(in.InterestedFeatures)
	if err != nil {
		return fmt.Errorf("encode onboarding interests: %w", err)
	}
	now := stamp()
	var completedAt *string
	if in.Completed {
		completedAt = &now
	}
	_, err = r.DB.Exec(ctx, `INSERT INTO user_onboarding_answers
		(user_id,organization_id,interested_features,work_for,client_website_count,found_via,completed_at,gsc_nudge_dismissed_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8)
		ON CONFLICT (user_id) DO UPDATE SET
		organization_id=EXCLUDED.organization_id,
		interested_features=CASE WHEN $9 THEN EXCLUDED.interested_features ELSE user_onboarding_answers.interested_features END,
		work_for=CASE WHEN $10 THEN EXCLUDED.work_for ELSE user_onboarding_answers.work_for END,
		client_website_count=CASE WHEN $11 THEN EXCLUDED.client_website_count ELSE user_onboarding_answers.client_website_count END,
		found_via=CASE WHEN $12 THEN EXCLUDED.found_via ELSE user_onboarding_answers.found_via END,
		completed_at=CASE WHEN $13 THEN EXCLUDED.completed_at ELSE user_onboarding_answers.completed_at END,
		gsc_nudge_dismissed_at=CASE WHEN $13 THEN EXCLUDED.gsc_nudge_dismissed_at ELSE user_onboarding_answers.gsc_nudge_dismissed_at END,
		updated_at=EXCLUDED.updated_at`, userID, organizationID, features, in.WorkFor, in.ClientWebsiteCount, in.FoundVia,
		completedAt, now, in.InterestedFeatures != nil, in.WorkFor != nil, in.ClientWebsiteCount != nil, in.FoundVia != nil, in.Completed)
	if err != nil {
		return fmt.Errorf("save onboarding answers: %w", err)
	}
	return nil
}

func (r Repository) DismissGSCNudge(ctx context.Context, userID, organizationID string) error {
	now := stamp()
	_, err := r.DB.Exec(ctx, `INSERT INTO user_onboarding_answers (user_id,organization_id,gsc_nudge_dismissed_at,updated_at)
		VALUES ($1,$2,$3,$3) ON CONFLICT (user_id) DO UPDATE SET gsc_nudge_dismissed_at=EXCLUDED.gsc_nudge_dismissed_at,updated_at=EXCLUDED.updated_at`, userID, organizationID, now)
	if err != nil {
		return fmt.Errorf("dismiss GSC onboarding nudge: %w", err)
	}
	return nil
}
func stringPointer(value string) *string { return &value }
