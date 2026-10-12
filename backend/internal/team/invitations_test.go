package team

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input string
		want  string
		bad   bool
	}{
		{input: "  PERSON@Example.COM  ", want: "person@example.com"},
		{input: "person@example.com", want: "person@example.com"},
		{input: "Person <person@example.com>", bad: true},
		{input: "person@example.com, other@example.com", bad: true},
		{input: "missing-at", bad: true},
		{input: "person@", bad: true},
		{input: "", bad: true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := normalizeEmail(tc.input)
			if (err != nil) != tc.bad {
				t.Fatalf("normalizeEmail(%q) error = %v", tc.input, err)
			}
			if !tc.bad && got != tc.want {
				t.Fatalf("normalizeEmail(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRoleCanInvite(t *testing.T) {
	t.Parallel()
	for role, want := range map[string]bool{
		"owner": true, "admin": true, "member": false, "owner,member": true,
		"member, admin": true, "administrator": false, "": false,
	} {
		t.Run(role, func(t *testing.T) {
			if got := roleCanInvite(role); got != want {
				t.Fatalf("roleCanInvite(%q) = %v, want %v", role, got, want)
			}
		})
	}
}

func TestSendEmailPayloadAndFallbackInviterName(t *testing.T) {
	var payload map[string]any
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://app.loops.so/api/v1/transactional" {
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode body: %v", err)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header), Request: r}, nil
	})
	baseURL, err := url.Parse("https://app.example.com/base/")
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{
		LoopsAPIKey: "test-key", InvitationTemplateID: "invite-template",
		BaseURL: baseURL, Client: &http.Client{Transport: transport},
	}
	user := auth.User{ID: "u1", Email: "inviter@example.com", Name: "   "}
	if err := service.sendEmail(context.Background(), "invitee@example.com", "id/with space", "Workspace", user); err != nil {
		t.Fatal(err)
	}
	vars, ok := payload["dataVariables"].(map[string]any)
	if !ok {
		t.Fatalf("dataVariables has type %T", payload["dataVariables"])
	}
	if payload["transactionalId"] != "invite-template" || payload["email"] != "invitee@example.com" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if vars["inviterName"] != user.Email || vars["organizationName"] != "Workspace" || vars["inviteUrl"] != "https://app.example.com/base/accept-invitation/id%2Fwith%20space" {
		t.Fatalf("unexpected template variables: %#v", vars)
	}
}

func TestSendEmailRequiresConfigurationAndPropagatesProviderFailure(t *testing.T) {
	baseURL, _ := url.Parse("https://app.example.com")
	for _, service := range []*Service{
		{}, {LoopsAPIKey: "key", InvitationTemplateID: "template"},
		{LoopsAPIKey: "key", BaseURL: baseURL},
	} {
		err := service.sendEmail(context.Background(), "invitee@example.com", "id", "Workspace", auth.User{})
		if !errors.Is(err, ErrUpstreamUnavailable) {
			t.Fatalf("configuration error = %v", err)
		}
	}
	service := &Service{LoopsAPIKey: "key", InvitationTemplateID: "template", BaseURL: baseURL, Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway", Body: io.NopCloser(strings.NewReader("provider down")), Header: make(http.Header)}, nil
	})}}
	if err := service.sendEmail(context.Background(), "invitee@example.com", "id", "Workspace", auth.User{}); !errors.Is(err, ErrUpstreamUnavailable) || !strings.Contains(err.Error(), "send invitation email") {
		t.Fatalf("provider error = %v", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConsumeBudgetEnforcesEmailAndOrganizationLimitsAtomically(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_REDIS_URL must be set in CI")
		}
		t.Skip("TEST_REDIS_URL not set; skipping Redis integration test")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect to test Redis: %v", err)
	}
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	organizationID := fmt.Sprintf("team-test-%d", os.Getpid())
	prefix := "invite-sends:" + organizationID + ":"
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+"*").Result()
		if err == nil && len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
	})
	service := &Service{Redis: client, Now: func() time.Time { return day }}

	for attempt := 0; attempt < 5; attempt++ {
		if err := service.consumeBudget(ctx, organizationID, "same@example.com"); err != nil {
			t.Fatalf("allowed recipient attempt %d: %v", attempt+1, err)
		}
	}
	if err := service.consumeBudget(ctx, organizationID, "same@example.com"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("sixth email to same recipient = %v, want rate limit", err)
	}
	// A rejected recipient attempt must not consume the organization budget.
	if err := service.consumeBudget(ctx, organizationID, "other@example.com"); err != nil {
		t.Fatalf("rejected recipient consumed organization budget: %v", err)
	}
	for attempt := 0; attempt < 44; attempt++ {
		email := fmt.Sprintf("recipient-%d@example.com", attempt)
		if err := service.consumeBudget(ctx, organizationID, email); err != nil {
			t.Fatalf("organization attempt %d: %v", attempt+1, err)
		}
	}
	if err := service.consumeBudget(ctx, organizationID, "last@example.com"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("organization attempt over limit = %v, want rate limit", err)
	}
}

func TestConsumeBudgetRequiresRedis(t *testing.T) {
	service := &Service{}
	if err := service.consumeBudget(context.Background(), "org", "person@example.com"); err == nil {
		t.Fatal("expected unavailable limiter error")
	}
}
