package branding

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

const (
	testOrg    = "org_branding_a"
	testOrgB   = "org_branding_b"
	testUserID = "user_branding"
)

// validLogo is a 1x1 PNG data URL, the shortest thing the pattern accepts.
const validLogo = "data:image/png;base64,iVBORw0KGgo="

// jsonPayload marshals a payload literal, failing the test on a mistake.
func jsonPayload(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(b)
}

// fullPayload is the shape the settings form always sends: every key present,
// the two nullable ones possibly null.
func fullPayload(name, accent string, logo, website any) map[string]any {
	return map[string]any{
		"brandName":   name,
		"accentColor": accent,
		"logoDataUrl": logo,
		"websiteUrl":  website,
	}
}

func TestValidateAcceptsTheSettingsFormShape(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    Input
	}{
		{
			name:    "full branding",
			payload: fullPayload("Acme Digital", "#2563eb", validLogo, "https://acme.agency"),
			want: Input{
				BrandName:   "Acme Digital",
				AccentColor: "#2563eb",
				LogoDataURL: new(validLogo),
				WebsiteURL:  new("https://acme.agency"),
			},
		},
		{
			name:    "nullable fields null",
			payload: fullPayload("Acme", "#FFFFFF", nil, nil),
			want:    Input{BrandName: "Acme", AccentColor: "#FFFFFF"},
		},
		{
			name:    "name is trimmed like zod's transform",
			payload: fullPayload("  Acme  ", "#000000", nil, nil),
			want:    Input{BrandName: "Acme", AccentColor: "#000000"},
		},
		{
			name:    "every accepted logo mime",
			payload: fullPayload("Acme", "#abcdef", "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=", nil),
			want:    Input{BrandName: "Acme", AccentColor: "#abcdef", LogoDataURL: new("data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=")},
		},
		{
			name:    "unicode name at the limit counts UTF-16 code units",
			payload: fullPayload(strings.Repeat("\u00e9", MaxNameChars), "#2563eb", nil, nil),
			want:    Input{BrandName: strings.Repeat("\u00e9", MaxNameChars), AccentColor: "#2563eb"},
		},
		{
			name:    "http website is allowed",
			payload: fullPayload("Acme", "#2563eb", nil, "http://localhost:3000"),
			want:    Input{BrandName: "Acme", AccentColor: "#2563eb", WebsiteURL: new("http://localhost:3000")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload brandingPayload
			if err := json.Unmarshal([]byte(jsonPayload(t, tt.payload)), &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got, err := payload.validate()
			if err != nil {
				t.Fatalf("validate() error = %v", err)
			}
			if !equalInput(got, tt.want) {
				t.Fatalf("validate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestValidateRejects(t *testing.T) {
	tooLongName := strings.Repeat("a", MaxNameChars+1)
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{name: "empty name", payload: fullPayload("", "#2563eb", nil, nil)},
		{name: "blank name trims to empty", payload: fullPayload("   ", "#2563eb", nil, nil)},
		{name: "name over the limit", payload: fullPayload(tooLongName, "#2563eb", nil, nil)},
		{name: "missing name key", payload: map[string]any{"accentColor": "#2563eb", "logoDataUrl": nil, "websiteUrl": nil}},
		{name: "missing accent key", payload: map[string]any{"brandName": "Acme", "logoDataUrl": nil, "websiteUrl": nil}},
		{name: "accent without hash", payload: fullPayload("Acme", "2563eb", nil, nil)},
		{name: "accent too short", payload: fullPayload("Acme", "#2563e", nil, nil)},
		{name: "accent with named colour", payload: fullPayload("Acme", "blue", nil, nil)},
		{name: "missing logo key", payload: map[string]any{"brandName": "Acme", "accentColor": "#2563eb", "websiteUrl": nil}},
		{name: "logo not a data url", payload: fullPayload("Acme", "#2563eb", "https://example.com/logo.png", nil)},
		{name: "logo mime not allowed", payload: fullPayload("Acme", "#2563eb", "data:image/gif;base64,R0lGODlh", nil)},
		{name: "logo not base64", payload: fullPayload("Acme", "#2563eb", "data:image/png;base64,<script>", nil)},
		{name: "logo over the limit", payload: fullPayload("Acme", "#2563eb", "data:image/png;base64,"+strings.Repeat("A", MaxLogoChars), nil)},
		{name: "missing website key", payload: map[string]any{"brandName": "Acme", "accentColor": "#2563eb", "logoDataUrl": nil}},
		{name: "website without a scheme", payload: fullPayload("Acme", "#2563eb", nil, "acme.agency")},
		{name: "website with a non-http scheme", payload: fullPayload("Acme", "#2563eb", nil, "ftp://acme.agency")},
		{name: "website with an empty host", payload: fullPayload("Acme", "#2563eb", nil, "https://")},
		{name: "website over the limit", payload: fullPayload("Acme", "#2563eb", nil, "https://acme.agency/"+strings.Repeat("a", MaxWebsiteChars))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload brandingPayload
			if err := json.Unmarshal([]byte(jsonPayload(t, tt.payload)), &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if _, err := payload.validate(); err == nil {
				t.Fatal("validate() accepted an invalid payload")
			}
		})
	}
}

func TestValidateCountsAstralNameAsTwoUnits(t *testing.T) {
	// 31 emoji are 62 UTF-16 code units, so zod's max(60) refused them; the
	// same payload must be refused here.
	var payload brandingPayload
	over := jsonPayload(t, fullPayload(strings.Repeat("\U0001F600", 31), "#2563eb", nil, nil))
	if err := json.Unmarshal([]byte(over), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, err := payload.validate(); err == nil {
		t.Fatal("validate() accepted 31 emoji, which is 62 UTF-16 units")
	}
}

func TestCanUpdateOrganization(t *testing.T) {
	tests := map[string]bool{
		"owner":        true,
		"admin":        true,
		"member":       false,
		"owner,admin":  true,
		"member,admin": true,
		"":             false,
		"admin,member": true,
		"other":        false,
	}
	for role, want := range tests {
		if got := canUpdateOrganization(role); got != want {
			t.Errorf("canUpdateOrganization(%q) = %v, want %v", role, got, want)
		}
	}
}

func TestSaveHandlerRejectsInvalidPayload(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := SaveHandler(logger, NewService(nil))

	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "not json", body: "{", want: http.StatusBadRequest},
		{name: "missing keys", body: `{}`, want: http.StatusBadRequest},
		{name: "bad accent", body: jsonPayload(t, fullPayload("Acme", "red", nil, nil)), want: http.StatusBadRequest},
		{name: "oversized body", body: `{"brandName":"` + strings.Repeat("a", maxBrandingBody) + `"}`, want: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/branding", strings.NewReader(tt.body))
			r = r.WithContext(auth.WithUser(r.Context(), auth.User{ID: testUserID, OrganizationID: testOrg, Role: "owner"}))
			h.ServeHTTP(rec, r)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.want, rec.Body.String())
			}
			if rec.Code != http.StatusOK {
				var env httpx.Envelope
				if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code == "" {
					t.Fatalf("error body = %s, want the standard envelope", rec.Body.String())
				}
			}
		})
	}
}

func TestSaveHandlerRequiresPermissionAndWorkspace(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := SaveHandler(logger, NewService(nil))
	body := jsonPayload(t, fullPayload("Acme", "#2563eb", nil, nil))

	tests := []struct {
		name string
		user auth.User
		want int
	}{
		{name: "member cannot save", user: auth.User{ID: testUserID, OrganizationID: testOrg, Role: "member"}, want: http.StatusForbidden},
		{name: "no active organization", user: auth.User{ID: testUserID, Role: "owner"}, want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/branding", strings.NewReader(body))
			r = r.WithContext(auth.WithUser(r.Context(), tt.user))
			h.ServeHTTP(rec, r)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestBrandingRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc := NewService(openBrandingDB(ctx, t))

	if _, found, err := svc.Get(ctx, testOrg); err != nil || found {
		t.Fatalf("Get() before save = found %v, err %v; want no row", found, err)
	}

	website := "https://acme.agency"
	logo := validLogo
	if err := svc.Save(ctx, testOrg, Input{
		BrandName:   "Acme Digital",
		AccentColor: "#2563eb",
		LogoDataURL: &logo,
		WebsiteURL:  &website,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, found, err := svc.Get(ctx, testOrg)
	if err != nil || !found {
		t.Fatalf("Get() = found %v, err %v", found, err)
	}
	if got.BrandName != "Acme Digital" || got.AccentColor != "#2563eb" ||
		got.LogoDataURL == nil || *got.LogoDataURL != logo ||
		got.WebsiteURL == nil || *got.WebsiteURL != website {
		t.Fatalf("Get() = %+v", got)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", got.UpdatedAt); err != nil {
		t.Fatalf("UpdatedAt = %q, want an ISO timestamp: %v", got.UpdatedAt, err)
	}

	// A second save replaces the row rather than adding one.
	if err := svc.Save(ctx, testOrg, Input{BrandName: "Acme", AccentColor: "#000000"}); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	got, _, err = svc.Get(ctx, testOrg)
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if got.BrandName != "Acme" || got.LogoDataURL != nil || got.WebsiteURL != nil {
		t.Fatalf("Get() after update = %+v, want the replacement row", got)
	}

	if err := svc.Reset(ctx, testOrg); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if _, found, err := svc.Get(ctx, testOrg); err != nil || found {
		t.Fatalf("Get() after reset = found %v, err %v; want no row", found, err)
	}
}

func TestBrandingIsScopedToOrganization(t *testing.T) {
	ctx := context.Background()
	svc := NewService(openBrandingDB(ctx, t))

	if err := svc.Save(ctx, testOrg, Input{BrandName: "Org A", AccentColor: "#111111"}); err != nil {
		t.Fatalf("Save(org A) error = %v", err)
	}
	if b, found, err := svc.Get(ctx, testOrgB); err != nil || found {
		t.Fatalf("Get(org B) = %+v, found %v, err %v; want org B untouched", b, found, err)
	}

	if err := svc.Reset(ctx, testOrgB); err != nil {
		t.Fatalf("Reset(org B) error = %v", err)
	}
	if _, found, err := svc.Get(ctx, testOrg); err != nil || !found {
		t.Fatalf("resetting org B removed org A's branding: found %v, err %v", found, err)
	}
}

func equalInput(a, b Input) bool {
	if a.BrandName != b.BrandName || a.AccentColor != b.AccentColor {
		return false
	}
	return equalPtr(a.LogoDataURL, b.LogoDataURL) && equalPtr(a.WebsiteURL, b.WebsiteURL)
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// openBrandingDB connects to TEST_DATABASE_URL, applies the legacy schema and
// adds organization_branding, which the shared fixture predates.
func openBrandingDB(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	pool, err := pgdb.Open(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	schema, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS organization_branding (
			organization_id text PRIMARY KEY REFERENCES organization (id) ON DELETE CASCADE,
			brand_name text NOT NULL,
			accent_color text NOT NULL,
			logo_data_url text,
			website_url text,
			updated_at text NOT NULL
		)`); err != nil {
		t.Fatalf("create organization_branding: %v", err)
	}
	for _, id := range []string{testOrg, testOrgB} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO organization (id, name, slug, created_at) VALUES ($1, $1, $1, now())
			 ON CONFLICT (id) DO NOTHING`, id); err != nil {
			t.Fatalf("seed organization %s: %v", id, err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM organization_branding WHERE organization_id = ANY($1)`,
		[]string{testOrg, testOrgB}); err != nil {
		t.Fatalf("clear branding: %v", err)
	}
	return pool
}
