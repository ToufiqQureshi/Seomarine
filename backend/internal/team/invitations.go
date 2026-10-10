// Package team owns team invitation creation and delivery.
package team

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

var (
	ErrForbidden           = errors.New("team invitation forbidden")
	ErrInvalidEmail        = errors.New("invalid email address")
	ErrConflict            = errors.New("invitee is already a member")
	ErrRateLimited         = errors.New("invitation email limit reached")
	ErrUpstreamUnavailable = errors.New("invitation email provider unavailable")
	ErrTooManyPending      = errors.New("organization invitation limit reached")
)

const invitationTTL = 7 * 24 * time.Hour

// Service creates pending membership invitations, applies daily send limits,
// and delivers invitation email through Loops.
type Service struct {
	DB                   *pgxpool.Pool
	Redis                *redis.Client
	LoopsAPIKey          string
	InvitationTemplateID string
	BaseURL              *url.URL
	Client               *http.Client
	Now                  func() time.Time
}

// Invite creates or refreshes a pending invitation and sends its email.
func (s *Service) Invite(ctx context.Context, user auth.User, email string) (string, error) {
	if !roleCanInvite(user.Role) || user.OrganizationID == "" {
		return "", ErrForbidden
	}
	address, err := normalizeEmail(email)
	if err != nil {
		return "", err
	}
	if err := s.consumeBudget(ctx, user.OrganizationID, address); err != nil {
		return "", err
	}
	now := s.now().UTC()
	id, organizationName, err := s.saveInvitation(ctx, user, address, now)
	if err != nil {
		return "", err
	}
	if err := s.sendEmail(ctx, address, id, organizationName, user); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) saveInvitation(ctx context.Context, user auth.User, email string, now time.Time) (string, string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin team invitation: %w", err)
	}
	defer tx.Rollback(ctx)
	var organizationName string
	if err := tx.QueryRow(ctx, `SELECT name FROM organization WHERE id=$1 FOR UPDATE`, user.OrganizationID).Scan(&organizationName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrForbidden
		}
		return "", "", fmt.Errorf("lock invitation organization: %w", err)
	}
	var inviterRole string
	if err := tx.QueryRow(ctx, `SELECT role FROM member WHERE organization_id=$1 AND user_id=$2 FOR UPDATE`, user.OrganizationID, user.ID).Scan(&inviterRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrForbidden
		}
		return "", "", fmt.Errorf("verify inviter membership: %w", err)
	}
	if !roleCanInvite(inviterRole) {
		return "", "", ErrForbidden
	}
	var isMember bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM member m JOIN "user" u ON u.id=m.user_id WHERE m.organization_id=$1 AND lower(u.email)=lower($2))`, user.OrganizationID, email).Scan(&isMember); err != nil {
		return "", "", fmt.Errorf("check invitee membership: %w", err)
	}
	if isMember {
		return "", "", ErrConflict
	}
	var invitationID string
	err = tx.QueryRow(ctx, `SELECT id FROM invitation WHERE organization_id=$1 AND lower(email)=lower($2) AND status='pending' AND expires_at>now() ORDER BY created_at DESC LIMIT 1`, user.OrganizationID, email).Scan(&invitationID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("find pending invitation: %w", err)
	}
	expires := now.Add(invitationTTL)
	if errors.Is(err, pgx.ErrNoRows) {
		var pending int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM invitation WHERE organization_id=$1 AND status='pending' AND expires_at>now()`, user.OrganizationID).Scan(&pending); err != nil {
			return "", "", fmt.Errorf("count pending invitations: %w", err)
		}
		if pending >= 20 {
			return "", "", ErrTooManyPending
		}
		invitationID, err = ids.New()
		if err != nil {
			return "", "", err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO invitation (id,organization_id,email,role,status,expires_at,created_at,inviter_id) VALUES ($1,$2,$3,'admin','pending',$4,$5,$6)`, invitationID, user.OrganizationID, email, expires, now, user.ID); err != nil {
			return "", "", fmt.Errorf("create team invitation: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `UPDATE invitation SET role='admin',expires_at=$1,inviter_id=$2 WHERE id=$3 AND organization_id=$4 AND status='pending'`, expires, user.ID, invitationID, user.OrganizationID); err != nil {
		return "", "", fmt.Errorf("refresh team invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("commit team invitation: %w", err)
	}
	return invitationID, organizationName, nil
}

func (s *Service) consumeBudget(ctx context.Context, organizationID, email string) error {
	if s.Redis == nil {
		return errors.New("invitation rate limiter unavailable")
	}
	day := s.now().UTC().Format("2006-01-02")
	for _, entry := range []struct {
		key   string
		limit int64
	}{{"invite-sends:" + organizationID + ":" + day, 50}, {"invite-sends:" + organizationID + ":" + email + ":" + day, 5}} {
		count, err := s.Redis.Eval(ctx, `local current=tonumber(redis.call('GET',KEYS[1]) or '0'); if current>=tonumber(ARGV[1]) then return current end; local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],86400) end; return n`, []string{entry.key}, entry.limit).Int64()
		if err != nil {
			return fmt.Errorf("consume invitation send budget: %w", err)
		}
		if count > entry.limit {
			return ErrRateLimited
		}
	}
	return nil
}

func (s *Service) sendEmail(ctx context.Context, email, invitationID, organizationName string, user auth.User) error {
	if strings.TrimSpace(s.LoopsAPIKey) == "" || strings.TrimSpace(s.InvitationTemplateID) == "" || s.BaseURL == nil {
		return ErrUpstreamUnavailable
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	inviteURL := strings.TrimRight(s.BaseURL.String(), "/") + "/accept-invitation/" + url.PathEscape(invitationID)
	inviterName := strings.TrimSpace(user.Name)
	if inviterName == "" {
		inviterName = user.Email
	}
	body, err := json.Marshal(map[string]any{"transactionalId": s.InvitationTemplateID, "email": email, "addToAudience": false, "dataVariables": map[string]string{"appName": "Seomarine", "inviteUrl": inviteURL, "organizationName": organizationName, "inviterName": inviterName, "inviterEmail": user.Email}})
	if err != nil {
		return fmt.Errorf("encode invitation email: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://app.loops.so/api/v1/transactional", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build invitation email request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.LoopsAPIKey)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send invitation email: %w", errors.Join(ErrUpstreamUnavailable, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Loops transactional invitation returned %s: %w", resp.Status, ErrUpstreamUnavailable)
	}
	return nil
}

func normalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Address != trimmed || !strings.Contains(parsed.Address, "@") {
		return "", ErrInvalidEmail
	}
	return strings.ToLower(trimmed), nil
}

func roleCanInvite(role string) bool {
	for _, part := range strings.Split(role, ",") {
		if strings.TrimSpace(part) == "owner" || strings.TrimSpace(part) == "admin" {
			return true
		}
	}
	return false
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
