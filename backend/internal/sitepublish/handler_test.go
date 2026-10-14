package sitepublish

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

type planStub struct {
	allowed bool
	err     error
}

func (p planStub) HasPaidPlan(context.Context, string) (bool, error) { return p.allowed, p.err }

// testMux mounts the routes with a fixed signed-in user who has access to
// project "p" of organization "org".
func testMux(service *Service, plans PaidPlans, user auth.User) *http.ServeMux {
	mux := http.NewServeMux()
	Mount(mux, Deps{
		Service: service, Plans: plans,
		WithSession: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
			})
		},
		WithProjectAccess: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org")))
			})
		},
	})
	return mux
}

func post(t *testing.T, mux *http.ServeMux, path, body string) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body)))
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	if strings.Contains(recorder.Body.String(), testPassword) {
		t.Fatalf("%s answered with the password: %s", path, recorder.Body.String())
	}
	return recorder.Code, decoded
}

func errorCode(body map[string]any) string {
	if inner, ok := body["error"].(map[string]any); ok {
		code, _ := inner["code"].(string)
		return code
	}
	code, _ := body["code"].(string)
	return code
}

var owner = auth.User{ID: "user-1", OrganizationID: "org", Role: "owner"}

func connectBody(siteURL, password string) string {
	body, _ := json.Marshal(ConnectInput{Platform: WordPressPlatform, SiteURL: siteURL, Username: testUser, ApplicationPassword: password})
	return string(body)
}

func TestHandlerConnectPublishAndDisconnect(t *testing.T) {
	wp := newFakeWordPress(t)
	service, _ := newTestService(t, wp, memoryDrafts{"d": {ID: "d", ProjectID: "p", Title: "Hello", Content: "Body"}})
	mux := testMux(service, planStub{allowed: true}, owner)

	if status, body := post(t, mux, "/api/v1/projects/p/site-connection/get", "{}"); status != http.StatusOK || body["connected"] != false {
		t.Fatalf("get before connecting = %d %v", status, body)
	}
	if status, body := post(t, mux, "/api/v1/projects/p/content/drafts/d/publish", `{"status":"draft"}`); status != http.StatusConflict || errorCode(body) != "site_not_connected" {
		t.Fatalf("publish before connecting = %d %v", status, body)
	}
	if status, body := post(t, mux, "/api/v1/projects/p/site-connection/save", connectBody(wp.server.URL, "wrong")); status != http.StatusUnprocessableEntity || errorCode(body) != "bad_credentials" {
		t.Fatalf("connect with a wrong password = %d %v", status, body)
	}
	status, body := post(t, mux, "/api/v1/projects/p/site-connection/save", connectBody(wp.server.URL, testPassword))
	if status != http.StatusOK || body["connected"] != true || body["siteUrl"] != wp.server.URL || body["siteName"] != "Test Blog" {
		t.Fatalf("connect = %d %v", status, body)
	}
	if _, has := body["secretCiphertext"]; has {
		t.Fatal("the encrypted secret reached the browser")
	}
	status, body = post(t, mux, "/api/v1/projects/p/content/drafts/d/publish", `{"status":"publish"}`)
	if status != http.StatusOK || body["published"] != true || body["status"] != "publish" || !strings.HasPrefix(body["url"].(string), wp.server.URL) {
		t.Fatalf("publish = %d %v", status, body)
	}
	if status, body := post(t, mux, "/api/v1/projects/p/content/drafts/d/publication/get", "{}"); status != http.StatusOK || body["published"] != true {
		t.Fatalf("publication = %d %v", status, body)
	}
	if status, body := post(t, mux, "/api/v1/projects/p/content/drafts/missing/publication/get", "{}"); status != http.StatusOK || body["published"] != false {
		t.Fatalf("publication of an unpublished draft = %d %v", status, body)
	}
	if status, _ := post(t, mux, "/api/v1/projects/p/content/drafts/missing/publish", `{"status":"draft"}`); status != http.StatusNotFound {
		t.Fatalf("publish a missing draft = %d", status)
	}
	if status, _ := post(t, mux, "/api/v1/projects/p/site-connection/delete", "{}"); status != http.StatusOK {
		t.Fatalf("disconnect = %d", status)
	}
	if _, body := post(t, mux, "/api/v1/projects/p/site-connection/get", "{}"); body["connected"] != false {
		t.Fatalf("still connected after disconnect: %v", body)
	}
}

func TestHandlerOnlyOwnersAndAdminsManageTheConnection(t *testing.T) {
	wp := newFakeWordPress(t)
	service, store := newTestService(t, wp, nil)
	for name, user := range map[string]auth.User{
		"member":                     {ID: "u", OrganizationID: "org", Role: "member"},
		"owner of another workspace": {ID: "u", OrganizationID: "other-org", Role: "owner"},
	} {
		mux := testMux(service, planStub{allowed: true}, user)
		if status, _ := post(t, mux, "/api/v1/projects/p/site-connection/save", connectBody(wp.server.URL, testPassword)); status != http.StatusForbidden {
			t.Errorf("%s connect = %d, want 403", name, status)
		}
		if status, _ := post(t, mux, "/api/v1/projects/p/site-connection/delete", "{}"); status != http.StatusForbidden {
			t.Errorf("%s disconnect = %d, want 403", name, status)
		}
	}
	if _, err := store.Connection(t.Context(), "p"); !errors.Is(err, ErrNotConnected) {
		t.Fatal("a member connected the site")
	}
	admin := testMux(service, planStub{allowed: true}, auth.User{ID: "a", OrganizationID: "org", Role: "admin"})
	if status, body := post(t, admin, "/api/v1/projects/p/site-connection/save", connectBody(wp.server.URL, testPassword)); status != http.StatusOK {
		t.Fatalf("admin connect = %d %v", status, body)
	}
}

func TestHandlerPlanGate(t *testing.T) {
	wp := newFakeWordPress(t)
	service, _ := newTestService(t, wp, memoryDrafts{"d": {ID: "d", ProjectID: "p", Title: "Hello", Content: "Body"}})
	connect(t, service, wp, "p")
	free := testMux(service, planStub{allowed: false}, owner)
	if status, body := post(t, free, "/api/v1/projects/p/site-connection/save", connectBody(wp.server.URL, testPassword)); status != http.StatusPaymentRequired || errorCode(body) != "plan_required" {
		t.Errorf("free connect = %d %v", status, body)
	}
	if status, body := post(t, free, "/api/v1/projects/p/content/drafts/d/publish", `{"status":"draft"}`); status != http.StatusPaymentRequired || errorCode(body) != "plan_required" {
		t.Errorf("free publish = %d %v", status, body)
	}
	// A workspace that stopped paying can still disconnect and read.
	if status, _ := post(t, free, "/api/v1/projects/p/site-connection/get", "{}"); status != http.StatusOK {
		t.Errorf("free get = %d", status)
	}
	broken := testMux(service, planStub{err: errors.New("db down")}, owner)
	if status, _ := post(t, broken, "/api/v1/projects/p/content/drafts/d/publish", `{"status":"draft"}`); status != http.StatusInternalServerError {
		t.Errorf("plan check failing = %d, want 500 (fail closed)", status)
	}
	if posts, _, _ := wp.snapshot(); len(posts) != 0 {
		t.Errorf("a refused publish reached the site: %d posts", len(posts))
	}
}

func TestHandlerRejectsMalformedRequests(t *testing.T) {
	wp := newFakeWordPress(t)
	service, _ := newTestService(t, wp, nil)
	mux := testMux(service, nil, owner)
	for path, body := range map[string]string{
		"/api/v1/projects/p/site-connection/get":              `{"x":1}`,
		"/api/v1/projects/p/site-connection/save":             `{"platform":"wordpress","extra":true}`,
		"/api/v1/projects/p/content/drafts/d/publish":         `not json`,
		"/api/v1/projects/p/site-connection/delete":           ``,
		"/api/v1/projects/p/content/drafts/d/publication/get": `[]`,
	} {
		if status, _ := post(t, mux, path, body); status != http.StatusBadRequest {
			t.Errorf("%s with %q = %d, want 400", path, body, status)
		}
	}
	unconfigured := testMux(nil, nil, owner)
	if status, _ := post(t, unconfigured, "/api/v1/projects/p/site-connection/get", "{}"); status != http.StatusServiceUnavailable {
		t.Errorf("unconfigured = %d, want 503", status)
	}
}
