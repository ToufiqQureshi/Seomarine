package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

// project is a row of the legacy projects table, shaped for the MCP tools.
type project struct {
	ID             string
	OrganizationID string
	Name           string
	Domain         *string
	LocationCode   int
	LanguageCode   string
	CreatedAt      string
}

// listProjects returns an organization's active projects, newest first. The
// ordering matches the legacy repository (created_at desc, id desc).
func listProjects(ctx context.Context, db *pgxpool.Pool, organizationID string) ([]project, error) {
	rows, err := db.Query(ctx, `
		SELECT id, organization_id, name, domain, location_code, language_code, created_at
		FROM projects WHERE organization_id = $1 AND archived_at IS NULL
		ORDER BY created_at DESC, id DESC`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// getProjectWithOrganization returns a project by id alone, with the org that
// owns it. Used by user-scoped credentials to derive the authorization org.
func getProjectWithOrganization(ctx context.Context, db *pgxpool.Pool, projectID string) (*project, bool, error) {
	row := db.QueryRow(ctx, `
		SELECT id, organization_id, name, domain, location_code, language_code, created_at
		FROM projects WHERE id = $1 AND archived_at IS NULL`, projectID)
	p, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &p, true, nil
}

// getProjectForOrganization returns a project only when it belongs to org.
func getProjectForOrganization(ctx context.Context, db *pgxpool.Pool, organizationID, projectID string) (*project, bool, error) {
	row := db.QueryRow(ctx, `
		SELECT id, organization_id, name, domain, location_code, language_code, created_at
		FROM projects WHERE id = $1 AND organization_id = $2 AND archived_at IS NULL`, projectID, organizationID)
	p, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &p, true, nil
}

// createProject inserts a project. domain and market are optional: an omitted
// market keeps the column defaults (2840/en).
func createProject(ctx context.Context, db *pgxpool.Pool, organizationID, name string, domain *string, market *market.Pair) (project, error) {
	id, err := ids.New()
	if err != nil {
		return project{}, err
	}
	locationCode := marketDefaultLocationCode
	languageCode := "en"
	if market != nil {
		locationCode = market.LocationCode
		languageCode = market.LanguageCode
	}
	row := db.QueryRow(ctx, `
		INSERT INTO projects (id, organization_id, name, domain, location_code, language_code)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, organization_id, name, domain, location_code, language_code, created_at`,
		id, organizationID, name, domain, locationCode, languageCode)
	p, err := scanProject(row)
	if err != nil {
		return project{}, err
	}
	return p, nil
}

const marketDefaultLocationCode = market.DefaultLocationCode

// scanProject reads a project from a pgx row.
func scanProject(row pgx.Row) (project, error) {
	var p project
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Domain, &p.LocationCode, &p.LanguageCode, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return project{}, err
		}
		return project{}, fmt.Errorf("scan project: %w", err)
	}
	return p, nil
}

// resolveMarketInput mirrors the legacy rule: a location with no language snaps
// to that location's default language; a language with no location is rejected
// by the schema before it reaches here.
func resolveMarketInput(locationCode *int, languageCode *string) (*market.Pair, error) {
	if locationCode == nil {
		return nil, nil
	}
	if _, ok := market.Lookup(*locationCode); !ok {
		return nil, fmt.Errorf("unsupported location code %d", *locationCode)
	}
	language := market.LanguageCode(*locationCode)
	if languageCode != nil && *languageCode != "" {
		language = *languageCode
	}
	if !market.IsSupportedLanguageCode(language) {
		return nil, fmt.Errorf("unsupported language code %q", language)
	}
	return &market.Pair{LocationCode: *locationCode, LanguageCode: language}, nil
}

// normalizeProjectDomain lowercases the bare host and strips a scheme, www,
// path and query, so junk fails at save time. It is a deliberately simpler
// validator than the legacy public-suffix check (see the migration doc).
func normalizeProjectDomain(domain string) (*string, error) {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return nil, nil
	}
	candidate := trimmed
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return nil, fmt.Errorf("invalid domain %q", trimmed)
	}
	host := strings.ToLower(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")
	if host == "" || !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return nil, fmt.Errorf("invalid domain %q", trimmed)
	}
	return &host, nil
}
