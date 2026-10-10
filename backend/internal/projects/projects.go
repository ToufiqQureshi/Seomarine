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
	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

var ErrNotFound = errors.New("project not found")
var ErrConflict = errors.New("project conflict")
var ErrForbidden = errors.New("project operation forbidden")

type Error struct{ Code, Message string }

func (e *Error) Error() string        { return e.Message }
func fail(code, message string) error { return &Error{Code: code, Message: message} }

type Project struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Domain       *string `json:"domain"`
	LocationCode int     `json:"locationCode"`
	LanguageCode string  `json:"languageCode"`
	CreatedAt    string  `json:"createdAt"`
}
type Input struct {
	Name         string  `json:"name"`
	Domain       *string `json:"domain"`
	LocationCode *int    `json:"locationCode"`
	LanguageCode *string `json:"languageCode"`
}
type Store interface {
	List(context.Context, string, bool) ([]Project, error)
	Get(context.Context, string, string, bool) (Project, error)
	Create(context.Context, string, Input) (Project, error)
	Update(context.Context, string, string, Input) (Project, error)
	Archive(context.Context, string, string, string) error
	Restore(context.Context, string, string) error
	Count(context.Context, string) (int, error)
	EnsureDefault(context.Context, string) error
	Role(context.Context, string, string) (string, error)
}
type Service struct {
	Store Store
	Now   func() time.Time
}

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
func (s *Service) Get(ctx context.Context, org, id string) (Project, error) {
	return s.Store.Get(ctx, org, id, false)
}
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
	return s.Store.Create(ctx, org, in)
}
func (s *Service) Update(ctx context.Context, org, id string, in Input) (Project, error) {
	if err := validateName(in.Name); err != nil {
		return Project{}, err
	}
	if err := normalize(&in); err != nil {
		return Project{}, err
	}
	return s.Store.Update(ctx, org, id, in)
}
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
func (s *Service) Restore(ctx context.Context, org, user, id string) error {
	if err := s.requireAdmin(ctx, org, user); err != nil {
		return err
	}
	return s.Store.Restore(ctx, org, id)
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

type Repository struct{ DB *pgxpool.Pool }

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
func (r Repository) Count(ctx context.Context, org string) (int, error) {
	var n int
	err := r.DB.QueryRow(ctx, `SELECT count(*) FROM projects WHERE organization_id=$1 AND archived_at IS NULL`, org).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count projects: %w", err)
	}
	return n, nil
}
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
func (r Repository) Archive(ctx context.Context, org, id, at string) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
