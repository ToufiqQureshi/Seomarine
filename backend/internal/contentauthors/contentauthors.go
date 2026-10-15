// Package contentauthors stores per-project author profiles. They feed the
// WordPress byline and the Article schema when a content-editor draft is
// published; they are not WordPress or Google users.
package contentauthors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/ids"
	"github.com/toufiqqureshi/uplumo/backend/internal/sitepublish"
)

const (
	// MaxNameLength is the maximum allowed author name length.
	MaxNameLength = 120
	// MaxRoleLength is the maximum allowed author role length.
	MaxRoleLength = 120
	// MaxBioLength is the maximum allowed author bio length.
	MaxBioLength = 300
	// MaxURLLength is the maximum allowed photo or profile link length.
	MaxURLLength = 500
	// ListLimit is the number of authors returned by one list request.
	ListLimit = 50
)

// ErrNotFound means the author does not exist or belongs to another project.
var ErrNotFound = errors.New("author not found")

// ValidationError describes invalid author input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// Author is one saved author profile.
type Author struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Bio       string `json:"bio"`
	PhotoURL  string `json:"photoUrl,omitempty"`
	// ProfileURL is an optional persona link (https only by policy). Search
	// engines grow more trust in authors when it points back at the site
	// being written for.
	ProfileURL string    `json:"profileUrl,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Input is what the client sends to create or update an author.
type Input struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	Bio        string `json:"bio"`
	PhotoURL   string `json:"photoUrl"`
	ProfileURL string `json:"profileUrl"`
}

// Repository stores author profiles in Postgres.
type Repository struct{ DB *pgxpool.Pool }

const authorColumns = `id, project_id, name, role, bio, photo_url, profile_url, created_at, updated_at`

// List returns a project's authors, oldest first, so the picker order is
// stable.
func (r Repository) List(ctx context.Context, projectID string) ([]Author, error) {
	rows, err := r.DB.Query(ctx, `SELECT `+authorColumns+`
		FROM go_content_authors WHERE project_id=$1 ORDER BY created_at, id LIMIT $2`, projectID, ListLimit)
	if err != nil {
		return nil, fmt.Errorf("list content authors: %w", err)
	}
	authors, err := pgx.CollectRows(rows, scanAuthorRow)
	if err != nil {
		return nil, fmt.Errorf("read content authors: %w", err)
	}
	return authors, nil
}

// Get returns one project author, or ErrNotFound.
func (r Repository) Get(ctx context.Context, projectID, id string) (Author, error) {
	var author Author
	err := r.DB.QueryRow(ctx, `SELECT `+authorColumns+`
		FROM go_content_authors WHERE project_id=$1 AND id=$2`, projectID, id).
		Scan(&author.ID, &author.ProjectID, &author.Name, &author.Role, &author.Bio, &author.PhotoURL, &author.ProfileURL, &author.CreatedAt, &author.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Author{}, ErrNotFound
	}
	if err != nil {
		return Author{}, fmt.Errorf("get content author: %w", err)
	}
	return author, nil
}

// Save creates or updates a project author. Saving against another project's
// author answers ErrNotFound, never a partial edit.
func (r Repository) Save(ctx context.Context, projectID, id string, input Input) (Author, error) {
	if err := validate(input); err != nil {
		return Author{}, err
	}
	name := strings.TrimSpace(input.Name)
	role := strings.TrimSpace(input.Role)
	bio := strings.TrimSpace(input.Bio)
	photo := strings.TrimSpace(input.PhotoURL)
	profile := strings.TrimSpace(input.ProfileURL)
	if id != "" {
		var author Author
		err := r.DB.QueryRow(ctx, `UPDATE go_content_authors SET name=$3, role=$4, bio=$5,
			photo_url=$6, profile_url=$7, updated_at=now() WHERE project_id=$1 AND id=$2 RETURNING `+authorColumns,
			projectID, id, name, role, bio, photo, profile).
			Scan(&author.ID, &author.ProjectID, &author.Name, &author.Role, &author.Bio, &author.PhotoURL, &author.ProfileURL, &author.CreatedAt, &author.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return Author{}, ErrNotFound
		}
		if err != nil {
			return Author{}, fmt.Errorf("update content author: %w", err)
		}
		return author, nil
	}
	newID, err := ids.New()
	if err != nil {
		return Author{}, fmt.Errorf("create content author id: %w", err)
	}
	author, err := scanOneAuthor(r.DB.QueryRow(ctx, `INSERT INTO go_content_authors
		(id, project_id, name, role, bio, photo_url, profile_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+authorColumns,
		newID, projectID, name, role, bio, photo, profile))
	if err != nil {
		return Author{}, fmt.Errorf("create content author: %w", err)
	}
	return author, nil
}

// Delete removes a project author. Drafts keep working: their author_id is
// set to NULL by the database.
func (r Repository) Delete(ctx context.Context, projectID, id string) error {
	tag, err := r.DB.Exec(ctx, `DELETE FROM go_content_authors WHERE project_id=$1 AND id=$2`, projectID, id)
	if err != nil {
		return fmt.Errorf("delete content author: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func validate(input Input) error {
	name := strings.TrimSpace(input.Name)
	bio := strings.TrimSpace(input.Bio)
	switch {
	case name == "":
		return &ValidationError{Message: "Enter the author's name."}
	case len([]rune(name)) > MaxNameLength:
		return &ValidationError{Message: fmt.Sprintf("The name must be at most %d characters.", MaxNameLength)}
	case len([]rune(strings.TrimSpace(input.Role))) > MaxRoleLength:
		return &ValidationError{Message: fmt.Sprintf("The role must be at most %d characters.", MaxRoleLength)}
	case len([]rune(bio)) > MaxBioLength:
		return &ValidationError{Message: fmt.Sprintf("The bio must be at most %d characters.", MaxBioLength)}
	case len(input.PhotoURL) > MaxURLLength || len(input.ProfileURL) > MaxURLLength:
		return &ValidationError{Message: "Photo and profile links must be at most 500 characters."}
	}
	if photo := strings.TrimSpace(input.PhotoURL); photo != "" {
		if err := validateHTTPURL(photo); err != nil {
			return &ValidationError{Message: "The photo link must start with https:// or http://."}
		}
	}
	if profile := strings.TrimSpace(input.ProfileURL); profile != "" {
		if err := validateHTTPURL(profile); err != nil {
			return &ValidationError{Message: "The profile link must start with https:// or http://."}
		}
	}
	return nil
}

// Author loads one profile shaped for the publishing flow, scoped to the
// project: sitepublish uses it for the byline and Article schema.
func (r Repository) Author(ctx context.Context, projectID, authorID string) (sitepublish.AuthorInfo, error) {
	author, err := r.Get(ctx, projectID, authorID)
	if err != nil {
		return sitepublish.AuthorInfo{}, err
	}
	return sitepublish.AuthorInfo{Name: author.Name, Role: author.Role, Bio: author.Bio, PhotoURL: author.PhotoURL, ProfileURL: author.ProfileURL}, nil
}

// validateHTTPURL accepts the same https-first rule the rest of Uplumo uses
// for outbound addresses: no user info, non-empty host, no spaces.
func validateHTTPURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || strings.ContainsAny(parsed.Host, " \t\r\n") {
		return fmt.Errorf("%q is not a usable link", raw)
	}
	return nil
}

// Mount registers the author routes. Every member of the project's workspace
// can manage authors: like drafts, they are editorial data.
func Mount(mux *http.ServeMux, deps Deps) {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	h := handler{deps}
	protect := func(next http.HandlerFunc) http.Handler { return deps.WithSession(deps.WithProjectAccess(next)) }
	mux.Handle("POST /api/v1/projects/{projectId}/content/authors/list", protect(h.list))
	mux.Handle("POST /api/v1/projects/{projectId}/content/authors/save", protect(h.save))
	mux.Handle("POST /api/v1/projects/{projectId}/content/authors/{authorId}/delete", protect(h.delete))
}

// Deps wires the author routes. Service is nil when the database is not
// configured; the routes then answer 503.
type Deps struct {
	Logger            *slog.Logger
	Service           *Repository
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

type handler struct{ Deps }

func (h handler) list(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	authors, err := h.Service.List(r.Context(), r.PathValue("projectId"))
	if err != nil {
		h.internal(w, r, "list content authors", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, authors)
}

func (h handler) save(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var input struct {
		ID string `json:"id"`
		Input
	}
	if !decode(w, r, &input) {
		return
	}
	author, err := h.Service.Save(r.Context(), r.PathValue("projectId"), input.ID, input.Input)
	var validationErr *ValidationError
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, author)
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "This author profile is no longer available.")
	case errors.As(err, &validationErr):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", validationErr.Error())
	default:
		h.internal(w, r, "save content author", err)
	}
}

func (h handler) delete(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	err := h.Service.Delete(r.Context(), r.PathValue("projectId"), r.PathValue("authorId"))
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "This author profile is no longer available.")
	default:
		h.internal(w, r, "delete content author", err)
	}
}

func (h handler) ready(w http.ResponseWriter) bool {
	if h.Service == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_authors_unavailable", "Author profiles are not available on this server.")
		return false
	}
	return true
}

func (h handler) internal(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if r.Context().Err() != nil {
		return
	}
	h.Logger.ErrorContext(r.Context(), operation, "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}

func scanAuthorRow(row pgx.CollectableRow) (Author, error) {
	return scanOneAuthor(row)
}

func scanOneAuthor(row pgx.Row) (Author, error) {
	var author Author
	err := row.Scan(&author.ID, &author.ProjectID, &author.Name, &author.Role, &author.Bio,
		&author.PhotoURL, &author.ProfileURL, &author.CreatedAt, &author.UpdatedAt)
	return author, err
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	trimmed := bytes.TrimSpace(body)
	if err != nil || len(trimmed) == 0 || trimmed[0] != '{' {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Send a JSON object.")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Send a JSON object with the expected fields.")
		return false
	}
	return true
}
