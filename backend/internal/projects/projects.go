package projects

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/backlinks"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/ids"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/market"
)

// ErrNotFound indicates that a project does not exist.
var ErrNotFound = errors.New("project not found")

// ErrConflict indicates that a project operation conflicts with current state.
var ErrConflict = errors.New("project conflict")

// ErrForbidden indicates that the caller cannot manage this project.
var ErrForbidden = errors.New("project operation forbidden")

// Error is a structured project service error.
type Error struct{ Code, Message string }

func (e *Error) Error() string        { return e.Message }
func fail(code, message string) error { return &Error{Code: code, Message: message} }

// Project is an SEO project within an organization.
type Project struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Domain       *string `json:"domain"`
	LocationCode int     `json:"locationCode"`
	LanguageCode string  `json:"languageCode"`
	CreatedAt    string  `json:"createdAt"`
}

// Input contains project fields supplied by a user.
type Input struct {
	Name         string  `json:"name"`
	Domain       *string `json:"domain"`
	LocationCode *int    `json:"locationCode"`
	LanguageCode *string `json:"languageCode"`
}

// Store defines project persistence operations.
type Store interface {
	List(context.Context, string, bool) ([]Project, error)
	Get(context.Context, string, string, bool) (Project, error)
	Create(context.Context, string, Input) (Project, error)
	CreateLimited(context.Context, string, Input, int) (Project, error)
	Update(context.Context, string, string, Input) (Project, error)
	Archive(context.Context, string, string, string) error
	Restore(context.Context, string, string) error
	RestoreLimited(context.Context, string, string, int) error
	Count(context.Context, string) (int, error)
	EnsureDefault(context.Context, string) error
	Role(context.Context, string, string) (string, error)
}

// Service applies project validation and authorization rules.
type Service struct {
	Store Store
	Now   func() time.Time
	Plans interface {
		SiteLimit(context.Context, string) (int, error)
	}
}

// List returns the organization projects, optionally ensuring a default exists.
func (s *Service) List(ctx context.Context, org string, archived bool, ensureOne bool) ([]Project, error) {
	if s == nil || s.Store == nil {
		return nil, fail("projects_unavailable", "Projects are not available on this server.")
	}
	rows, err := s.Store.List(ctx, org, archived)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 || archived || !ensureOne {
		return rows, nil
	}
	if err = s.Store.EnsureDefault(ctx, org); err != nil {
		return nil, err
	}
	return s.Store.List(ctx, org, false)
}

// Get returns an active project by organization and ID.
func (s *Service) Get(ctx context.Context, org, id string) (Project, error) {
	return s.Store.Get(ctx, org, id, false)
}

// Create validates and creates a project for an organization.
func (s *Service) Create(ctx context.Context, org, user string, in Input) (Project, error) {
	if err := s.requireAdmin(ctx, org, user); err != nil {
		return Project{}, err
	}
	if err := validateName(in.Name); err != nil {
		return Project{}, err
	}
	if err := normalize(&in); err != nil {
		return Project{}, err
	}
	limit, err := s.siteLimit(ctx, org)
	if err != nil {
		return Project{}, err
	}
	if limit > 0 {
		return s.Store.CreateLimited(ctx, org, in, limit)
	}
	return s.Store.Create(ctx, org, in)
}

// Update validates and saves project details.
func (s *Service) Update(ctx context.Context, org, id string, in Input) (Project, error) {
	if err := validateName(in.Name); err != nil {
		return Project{}, err
	}
	if err := normalize(&in); err != nil {
		return Project{}, err
	}
	return s.Store.Update(ctx, org, id, in)
}

// Archive removes a project from active project lists.
func (s *Service) Archive(ctx context.Context, org, user, id string) error {
	if err := s.requireAdmin(ctx, org, user); err != nil {
		return err
	}
	count, err := s.Store.Count(ctx, org)
	if err != nil {
		return err
	}
	if count <= 1 {
		return fail("CONFLICT", "You can't archive your only project.")
	}
	return s.Store.Archive(ctx, org, id, utc(s.now()))
}

// Restore an archived project for an organization.
func (s *Service) Restore(ctx context.Context, org, user, id string) error {
	if err := s.requireAdmin(ctx, org, user); err != nil {
		return err
	}
	limit, err := s.siteLimit(ctx, org)
	if err != nil {
		return err
	}
	if limit > 0 {
		return s.Store.RestoreLimited(ctx, org, id, limit)
	}
	return s.Store.Restore(ctx, org, id)
}

func (s *Service) siteLimit(ctx context.Context, org string) (int, error) {
	if s.Plans == nil {
		return 0, nil
	}
	return s.Plans.SiteLimit(ctx, org)
}
func (s *Service) requireAdmin(ctx context.Context, org, user string) error {
	role, err := s.Store.Role(ctx, org, user)
	if err != nil {
		return err
	}
	if role != "owner" && role != "admin" {
		return fail("FORBIDDEN", "You need an organization admin role to manage projects.")
	}
	return nil
}
func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(utf16.Encode([]rune(name))) > 120 {
		return fail("VALIDATION_ERROR", "Project name must be between 1 and 120 characters.")
	}
	return nil
}
func normalize(in *Input) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Domain != nil {
		domain := strings.TrimSpace(*in.Domain)
		if domain == "" {
			in.Domain = nil
		} else {
			normalized, err := backlinks.NormalizeProjectDomain(domain)
			if err != nil {
				return fail("VALIDATION_ERROR", "Enter a valid domain, like acme.com.")
			}
			in.Domain = &normalized
		}
	}
	if in.LanguageCode != nil && in.LocationCode == nil {
		return fail("VALIDATION_ERROR", "A language requires a location.")
	}
	if in.LocationCode != nil {
		location, ok := market.Lookup(*in.LocationCode)
		if !ok {
			return fail("VALIDATION_ERROR", "Unsupported DataForSEO location code.")
		}
		language := location.LanguageCode
		if in.LanguageCode != nil && *in.LanguageCode != "" {
			language = *in.LanguageCode
		}
		allowed := false
		for _, candidate := range market.LanguageOptions(location.Code) {
			if candidate.Code == language {
				allowed = true
				break
			}
		}
		if !allowed {
			return fail("VALIDATION_ERROR", "Language is not supported for this location.")
		}
		in.LanguageCode = &language
	}
	return nil
}
func utc(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// Repository stores projects in Postgres.
type Repository struct{ DB *pgxpool.Pool }

// List returns the organization projects, optionally including archived projects.
func (r Repository) List(ctx context.Context, org string, archived bool) ([]Project, error) {
	condition := "archived_at IS NULL"
	if archived {
		condition = "archived_at IS NOT NULL"
	}
	rows, err := r.DB.Query(ctx, `SELECT id,name,domain,location_code,language_code,created_at FROM projects WHERE organization_id=$1 AND `+condition+` ORDER BY created_at DESC,id DESC`, org)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Domain, &p.LocationCode, &p.LanguageCode, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns a project by organization and ID.
func (r Repository) Get(ctx context.Context, org, id string, archived bool) (Project, error) {
	condition := "archived_at IS NULL"
	if archived {
		condition = "archived_at IS NOT NULL"
	}
	var p Project
	err := r.DB.QueryRow(ctx, `SELECT id,name,domain,location_code,language_code,created_at FROM projects WHERE organization_id=$1 AND id=$2 AND `+condition, org, id).Scan(&p.ID, &p.Name, &p.Domain, &p.LocationCode, &p.LanguageCode, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

// ProjectMarket returns an active project's own domain (empty when unset)
// and its search location and language, for features that read search
// results for the project.
func (r Repository) ProjectMarket(ctx context.Context, org, id string) (string, int, string, error) {
	p, err := r.Get(ctx, org, id, false)
	if err != nil {
		return "", 0, "", err
	}
	domain := ""
	if p.Domain != nil {
		domain = *p.Domain
	}
	return domain, p.LocationCode, p.LanguageCode, nil
}

// Create inserts a project for an organization.
func (r Repository) Create(ctx context.Context, org string, in Input) (Project, error) {
	id, err := ids.New()
	if err != nil {
		return Project{}, err
	}
	loc, lang := market.DefaultLocationCode, "en"
	if in.LocationCode != nil {
		loc = *in.LocationCode
		lang = *in.LanguageCode
	}
	var p Project
	err = r.DB.QueryRow(ctx, `INSERT INTO projects(id,organization_id,name,domain,location_code,language_code) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,name,domain,location_code,language_code,created_at`, id, org, in.Name, in.Domain, loc, lang).Scan(&p.ID, &p.Name, &p.Domain, &p.LocationCode, &p.LanguageCode, &p.CreatedAt)
	if err != nil {
		if isUnique(err) {
			return Project{}, fail("CONFLICT", `A project named "Default" with no domain already exists.`)
		}
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// CreateLimited serializes site creation per organization so concurrent
// requests cannot exceed the plan's active-site limit.
func (r Repository) CreateLimited(ctx context.Context, org string, in Input, limit int) (Project, error) {
	if limit <= 0 {
		return r.Create(ctx, org, in)
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return Project{}, fmt.Errorf("begin limited project create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM organization WHERE id=$1 FOR UPDATE`, org); err != nil {
		return Project{}, fmt.Errorf("lock organization for project create: %w", err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM projects WHERE organization_id=$1 AND archived_at IS NULL`, org).Scan(&count); err != nil {
		return Project{}, fmt.Errorf("count active sites: %w", err)
	}
	if count >= limit {
		return Project{}, fail("LIMIT_REACHED", fmt.Sprintf("Your plan allows %d active website(s). Archive a site or upgrade your plan to add another.", limit))
	}
	id, err := ids.New()
	if err != nil {
		return Project{}, err
	}
	loc, lang := market.DefaultLocationCode, "en"
	if in.LocationCode != nil {
		loc, lang = *in.LocationCode, *in.LanguageCode
	}
	var p Project
	err = tx.QueryRow(ctx, `INSERT INTO projects(id,organization_id,name,domain,location_code,language_code) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,name,domain,location_code,language_code,created_at`, id, org, in.Name, in.Domain, loc, lang).Scan(&p.ID, &p.Name, &p.Domain, &p.LocationCode, &p.LanguageCode, &p.CreatedAt)
	if err != nil {
		if isUnique(err) {
			return Project{}, fail("CONFLICT", `A project named "Default" with no domain already exists.`)
		}
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Project{}, fmt.Errorf("commit project create: %w", err)
	}
	return p, nil
}

// Update saves project details.
func (r Repository) Update(ctx context.Context, org, id string, in Input) (Project, error) {
	var p Project
	err := r.DB.QueryRow(ctx, `UPDATE projects SET name=$3,domain=$4,location_code=COALESCE($5,location_code),language_code=COALESCE($6,language_code) WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL RETURNING id,name,domain,location_code,language_code,created_at`, org, id, in.Name, in.Domain, in.LocationCode, in.LanguageCode).Scan(&p.ID, &p.Name, &p.Domain, &p.LocationCode, &p.LanguageCode, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		if isUnique(err) {
			return Project{}, fail("CONFLICT", `A project named "Default" with no domain already exists.`)
		}
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	return p, nil
}

// Count returns the number of projects in an organization.
func (r Repository) Count(ctx context.Context, org string) (int, error) {
	var n int
	err := r.DB.QueryRow(ctx, `SELECT count(*) FROM projects WHERE organization_id=$1 AND archived_at IS NULL`, org).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count projects: %w", err)
	}
	return n, nil
}

// EnsureDefault creates the default project when an organization has none.
func (r Repository) EnsureDefault(ctx context.Context, org string) error {
	id, err := ids.New()
	if err != nil {
		return err
	}
	_, err = r.DB.Exec(ctx, `INSERT INTO projects(id,organization_id,name,domain) SELECT $1,$2,'Default',NULL WHERE NOT EXISTS(SELECT 1 FROM projects WHERE organization_id=$2 AND archived_at IS NULL) ON CONFLICT DO NOTHING`, id, org)
	if err != nil {
		return fmt.Errorf("create default project: %w", err)
	}
	return nil
}

// Archive marks a project archived and records the update time.
func (r Repository) Archive(ctx context.Context, org, id, at string) (retErr error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("rollback project archive: %w", err))
		}
	}()
	if _, err = tx.Exec(ctx, `SELECT id FROM organization WHERE id=$1 FOR UPDATE`, org); err != nil {
		return fmt.Errorf("lock organization for project archive: %w", err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM projects WHERE organization_id=$1 AND archived_at IS NULL`, org).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return fail("CONFLICT", "You can't archive your only project.")
	}
	tag, err := tx.Exec(ctx, `UPDATE projects SET archived_at=$3 WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL`, org, id, at)
	if err != nil {
		return fmt.Errorf("archive project: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// Restore makes an archived project active again.
func (r Repository) Restore(ctx context.Context, org, id string) error {
	tag, err := r.DB.Exec(ctx, `UPDATE projects SET archived_at=NULL WHERE organization_id=$1 AND id=$2 AND archived_at IS NOT NULL`, org, id)
	if err != nil {
		if isUnique(err) {
			return fail("CONFLICT", `An active project named "Default" with no domain already exists. Rename it first, then restore this one.`)
		}
		return fmt.Errorf("restore project: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

// RestoreLimited prevents archived sites from bypassing the current plan cap.
func (r Repository) RestoreLimited(ctx context.Context, org, id string, limit int) error {
	if limit <= 0 {
		return r.Restore(ctx, org, id)
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin limited project restore: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM organization WHERE id=$1 FOR UPDATE`, org); err != nil {
		return fmt.Errorf("lock organization for project restore: %w", err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM projects WHERE organization_id=$1 AND archived_at IS NULL`, org).Scan(&count); err != nil {
		return fmt.Errorf("count active sites: %w", err)
	}
	if count >= limit {
		return fail("LIMIT_REACHED", fmt.Sprintf("Your plan allows %d active website(s). Archive a site or upgrade your plan to restore this one.", limit))
	}
	tag, err := tx.Exec(ctx, `UPDATE projects SET archived_at=NULL WHERE organization_id=$1 AND id=$2 AND archived_at IS NOT NULL`, org, id)
	if err != nil {
		if isUnique(err) {
			return fail("CONFLICT", `An active project named "Default" with no domain already exists. Rename it first, then restore this one.`)
		}
		return fmt.Errorf("restore project: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// Role returns the user role within an organization.
func (r Repository) Role(ctx context.Context, org, user string) (string, error) {
	var role string
	err := r.DB.QueryRow(ctx, `SELECT role FROM member WHERE organization_id=$1 AND user_id=$2`, org, user).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", fmt.Errorf("load project manager role: %w", err)
	}
	return role, nil
}
func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
