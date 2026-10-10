package sam

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

type storeStub struct {
	listedProject  string
	listedUser     string
	createdProject string
	createdUser    string
	archivedAt     string
	archiveOK      bool
}

func (s *storeStub) ListForProject(_ context.Context, projectID, userID string) ([]Session, error) {
	s.listedProject, s.listedUser = projectID, userID
	return []Session{{ID: "session-1", Title: "New chat", CreatedAt: "created", UpdatedAt: "updated"}}, nil
}
func (s *storeStub) Create(_ context.Context, projectID, userID string) (Session, error) {
	s.createdProject, s.createdUser = projectID, userID
	return Session{ID: "session-2", Title: "New chat"}, nil
}
func (s *storeStub) Archive(_ context.Context, projectID, userID, sessionID, archivedAt string) (bool, error) {
	if projectID != "project-1" || userID != "user-1" || sessionID != "session-1" {
		return false, nil
	}
	s.archivedAt = archivedAt
	return s.archiveOK, nil
}

func TestServiceScopesSessionOperationsAndArchivesWithUTC(t *testing.T) {
	store := &storeStub{archiveOK: true}
	service := &Service{Store: store, Now: func() time.Time {
		return time.Date(2026, 10, 9, 21, 4, 5, 6_000_000, time.FixedZone("offset", 2*60*60))
	}}
	if _, err := service.List(context.Background(), "project-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if store.listedProject != "project-1" || store.listedUser != "user-1" {
		t.Fatalf("list scope = %s/%s", store.listedProject, store.listedUser)
	}
	if _, err := service.Create(context.Background(), "project-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if store.createdProject != "project-1" || store.createdUser != "user-1" {
		t.Fatalf("create scope = %s/%s", store.createdProject, store.createdUser)
	}
	if err := service.Archive(context.Background(), "project-1", "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	if store.archivedAt != "2026-10-09T19:04:05.006Z" {
		t.Fatalf("archivedAt = %q", store.archivedAt)
	}
}

func TestServiceHidesMissingOrArchivedSessions(t *testing.T) {
	service := &Service{Store: &storeStub{archiveOK: false}}
	if err := service.Archive(context.Background(), "project-1", "user-1", "session-1"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestMountRejectsUnknownFieldsAndScopesCreateToProjectMember(t *testing.T) {
	store := &storeStub{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithUser(r.Context(), auth.User{ID: "user-1"})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	withProject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1")))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: logger, Service: &Service{Store: store}, WithSession: withSession, WithProjectAccess: withProject})

	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/sam/sessions/list", strings.NewReader(`{"extra":true}`)))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", invalid.Code)
	}

	created := httptest.NewRecorder()
	mux.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/sam/sessions/create", strings.NewReader(`{}`)))
	if created.Code != http.StatusOK || created.Body.String() != "{\"id\":\"session-2\"}\n" {
		t.Fatalf("create response = %d %s", created.Code, created.Body.String())
	}
	if store.createdProject != "project-1" || store.createdUser != "user-1" {
		t.Fatalf("created scope = %s/%s", store.createdProject, store.createdUser)
	}
}
