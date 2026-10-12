package contenteditor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

type memoryStore struct {
	listProject   string
	saveProject   string
	saveInput     Input
	deleteProject string
	deleteID      string
}

type fakeWriterService struct {
	organizationID string
	input          WriterInput
	output         WriterOutput
	err            error
}

func (f *fakeWriterService) Generate(_ context.Context, organizationID string, input WriterInput) (WriterOutput, error) {
	f.organizationID, f.input = organizationID, input
	return f.output, f.err
}

func (s *memoryStore) List(_ context.Context, projectID string) ([]DraftSummary, error) {
	s.listProject = projectID
	return []DraftSummary{}, nil
}
func (s *memoryStore) Get(_ context.Context, projectID, id string) (Draft, error) {
	return Draft{ID: id, ProjectID: projectID, Content: "Draft body"}, nil
}
func (s *memoryStore) Save(_ context.Context, projectID string, input Input) (Draft, error) {
	s.saveProject, s.saveInput = projectID, input
	return Draft{ID: "draft-1", ProjectID: projectID, Title: input.Title}, nil
}
func (s *memoryStore) Delete(_ context.Context, projectID, id string) error {
	s.deleteProject, s.deleteID = projectID, id
	return nil
}

func TestHandlerValidatesBodiesAndKeepsOperationsProjectScoped(t *testing.T) {
	store := &memoryStore{}
	identity := func(next http.Handler) http.Handler { return next }
	mux := http.NewServeMux()
	Mount(mux, store, nil, identity, identity)

	request := func(method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
		return recorder
	}
	if got := request(http.MethodPost, "/api/v1/projects/p1/content/drafts/list", `{"extra":true}`); got.Code != http.StatusBadRequest {
		t.Fatalf("list with unexpected field status = %d, want 400", got.Code)
	}
	if got := request(http.MethodPost, "/api/v1/projects/p1/content/drafts/list", `{} {}`); got.Code != http.StatusBadRequest {
		t.Fatalf("list with trailing JSON status = %d, want 400", got.Code)
	}
	if got := request(http.MethodPost, "/api/v1/projects/p1/content/drafts/list", `null`); got.Code != http.StatusBadRequest {
		t.Fatalf("list with null body status = %d, want 400", got.Code)
	}
	if got := request(http.MethodPost, "/api/v1/projects/p1/content/drafts/list", `{}`); got.Code != http.StatusOK || store.listProject != "p1" {
		t.Fatalf("list response = %d %s; project = %q", got.Code, got.Body, store.listProject)
	}
	if got := request(http.MethodPost, "/api/v1/projects/p1/content/drafts/save", `{"title":"Hello","content":"Draft"}`); got.Code != http.StatusOK || store.saveProject != "p1" || store.saveInput.Title != "Hello" {
		t.Fatalf("save response = %d %s; project/input = %q/%+v", got.Code, got.Body, store.saveProject, store.saveInput)
	}
	if got := request(http.MethodPost, "/api/v1/projects/p1/content/drafts/d1/get", `{}`); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Draft body") {
		t.Fatalf("get response = %d %s", got.Code, got.Body)
	}
	if got := request(http.MethodPost, "/api/v1/projects/p1/content/drafts/d1/delete", `{}`); got.Code != http.StatusOK || store.deleteProject != "p1" || store.deleteID != "d1" {
		t.Fatalf("delete response = %d %s; project/id = %q/%q", got.Code, got.Body, store.deleteProject, store.deleteID)
	}
}

func TestHandlerRejectsOversizedPayload(t *testing.T) {
	identity := func(next http.Handler) http.Handler { return next }
	mux := http.NewServeMux()
	Mount(mux, &memoryStore{}, nil, identity, identity)
	body := `{"content":"` + strings.Repeat("x", maxRequestBody+1) + `"}`
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/content/drafts/save", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("oversized payload status = %d, want 400", recorder.Code)
	}
}

func TestWriterHandlerRequiresProjectOrganizationAndValidRequest(t *testing.T) {
	writer := &fakeWriterService{output: WriterOutput{Content: "## Generated outline"}}
	identity := func(next http.Handler) http.Handler { return next }
	withOrganization := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-42")))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, &memoryStore{}, writer, identity, withOrganization)
	request := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/content/assistant/generate", strings.NewReader(body)))
		return recorder
	}
	if got := request(`{"action":"outline","extra":"no"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", got.Code)
	}
	if got := request(`{"action":"outline"}`); got.Code != http.StatusOK || writer.organizationID != "org-42" || writer.input.Action != WriterOutline || !strings.Contains(got.Body.String(), "Generated outline") {
		t.Fatalf("generation response = %d %s; writer = %+v", got.Code, got.Body, writer)
	}

	withoutOrganization := http.NewServeMux()
	Mount(withoutOrganization, &memoryStore{}, writer, identity, identity)
	failed := httptest.NewRecorder()
	withoutOrganization.ServeHTTP(failed, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/content/assistant/generate", strings.NewReader(`{"action":"outline"}`)))
	if failed.Code != http.StatusNotFound {
		t.Fatalf("missing authorized organization status = %d, want 404", failed.Code)
	}
}

func TestWriterHandlerMapsUnavailableAndNoCreditErrors(t *testing.T) {
	identity := func(next http.Handler) http.Handler { return next }
	withOrganization := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-42")))
		})
	}
	for _, test := range []struct {
		name   string
		writer WriterService
		want   int
	}{
		{name: "missing provider", want: http.StatusServiceUnavailable},
		{name: "no credits", writer: &fakeWriterService{err: ErrWriterNoCredits}, want: http.StatusPaymentRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			Mount(mux, &memoryStore{}, test.writer, identity, withOrganization)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/content/assistant/generate", strings.NewReader(`{"action":"outline"}`)))
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body)
			}
		})
	}
}
