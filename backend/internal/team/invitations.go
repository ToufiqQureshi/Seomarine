// Package team owns team invitation creation and delivery.
package team

import (
	"context"
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
	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	loops "github.com/toufiqqureshi/uplumo/backend/internal/email"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/ids"
)

var (
	// ErrForbidden indicates the caller cannot manage team invitations.
	ErrForbidden = errors.New("team invitation forbidden")
	// ErrInvalidEmail indicates that an invitation email address is invalid.
	ErrInvalidEmail = errors.New("invalid email address")
	// ErrConflict indicates that the invitee already belongs to the organization.
	ErrConflict = errors.New("invitee is already a member")
	// ErrRateLimited indicates that the invitation email limit was reached.
	ErrRateLimited = errors.New("invitation email limit reached")
	// ErrUpstreamUnavailable indicates that the invitation email provider failed.
	ErrUpstreamUnavailable = errors.New("invitation email provider unavailable")
	// ErrSeatLimit indicates that the workspace's plan allows no more people.
	ErrSeatLimit = errors.New("plan member limit reached")
	// ErrTooManyPending indicates that the organization has reached its invitation limit.
	ErrTooManyPending = errors.New("organization invitation limit reached")
)

const invitationTTL = 7 * 24 * time.Hour

// Service creates pending membership invitations, applies daily send limits,
// and delivers invitation email through Loops.
type Service struct {
	// Limits says how many people a workspace's plan allows. Nil is no limit.
	Limits               MemberLimits
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
	seats := 0
	if s.Limits != nil {
		if seats, err = s.Limits.MemberLimit(ctx, user.OrganizationID); err != nil {
			return "", fmt.Errorf("check member limit: %w", err)
		}
	}
	now := s.now().UTC()
	id, organizationName, err := s.saveInvitation(ctx, user, address, now, seats)
	if err != nil {
		return "", err
	}
	if err := s.sendEmail(ctx, address, id, organizationName, user); err != nil {
		return "", err
	}
	return id, nil
}

// MemberLimits reports how many people an organization's plan allows; 0 is
// no limit.
type MemberLimits interface {
	MemberLimit(ctx context.Context, organizationID string) (int, error)
}

// saveInvitation stores the invitation. seats is the most people the
// workspace may hold, members and pending invitations together; 0 is no
// limit. It is checked under the organization lock, so two invitations
// cannot together pass it.
func (s *Service) saveInvitation(ctx context.Context, user auth.User, email string, now time.Time, seats int) (invitationID, organizationName string, retErr error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin team invitation: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("rollback team invitation: %w", err))
		}
	}()
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
		if seats > 0 {
			var members int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM member WHERE organization_id=$1`, user.OrganizationID).Scan(&members); err != nil {
				return "", "", fmt.Errorf("count members: %w", err)
			}
			if members+pending >= seats {
				return "", "", ErrSeatLimit
			}
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
	keys := []string{
		"invite-sends:" + organizationID + ":" + day,
		"invite-sends:" + organizationID + ":" + email + ":" + day,
	}
	allowed, err := s.Redis.Eval(ctx, `
		local organizationCount=tonumber(redis.call('GET',KEYS[1]) or '0')
		local recipientCount=tonumber(redis.call('GET',KEYS[2]) or '0')
		if organizationCount>=tonumber(ARGV[1]) or recipientCount>=tonumber(ARGV[2]) then return 0 end
		for i=1,2 do
			local n=redis.call('INCR',KEYS[i])
			if n==1 then redis.call('EXPIRE',KEYS[i],86400) end
		end
		return 1`, keys, 50, 5).Int64()
	if err != nil {
		return fmt.Errorf("consume invitation send budget: %w", err)
	}
	if allowed != 1 {
		return ErrRateLimited
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
	provider := loops.LoopsClient{APIKey: s.LoopsAPIKey, HTTP: client}
	if err := provider.SendTransactional(ctx, email, s.InvitationTemplateID, map[string]string{
		"appName":          "Uplumo",
		"inviteUrl":        inviteURL,
		"organizationName": organizationName,
		"inviterName":      inviterName,
		"inviterEmail":     user.Email,
	}); err != nil {
		return fmt.Errorf("send invitation email: %w", errors.Join(ErrUpstreamUnavailable, err))
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
