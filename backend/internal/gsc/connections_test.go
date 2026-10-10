package gsc

import (
	"context"
	"errors"
	"testing"
)

type connectionOpsStub struct {
	connection ProjectProperty
	getErr     error
	grants     []Grant
	hasGrant   bool
	canManage  bool
	upserted   ProjectProperty
	deleted    bool
}

func (s *connectionOpsStub) GetByProjectID(context.Context, string, string) (ProjectProperty, error) {
	return s.connection, s.getErr
}
func (s *connectionOpsStub) HasGrant(context.Context, string) (bool, error) { return s.hasGrant, nil }
func (s *connectionOpsStub) ListGrants(context.Context, string) ([]Grant, error) {
	return s.grants, nil
}
func (s *connectionOpsStub) CanManage(context.Context, string, string, string) (bool, error) {
	return s.canManage, nil
}
func (s *connectionOpsStub) Upsert(_ context.Context, _, _ string, connection ProjectProperty) (ProjectProperty, error) {
	s.upserted = connection
	connection.CreatedAt = "2026-01-02T03:04:05.000Z"
	return connection, nil
}
func (s *connectionOpsStub) Delete(context.Context, string, string) error {
	s.deleted = true
	return nil
}

type searchConsoleClientStub struct {
	sites []Site
	email string
	err   error
}

type inspectingConsoleClient struct {
	searchConsoleClientStub
	seen []string
	err  map[string]error
}

func (s *inspectingConsoleClient) InspectURL(_ context.Context, _, target, _ string) (map[string]any, error) {
	s.seen = append(s.seen, target)
	if err := s.err[target]; err != nil {
		return nil, err
	}
	return map[string]any{"status": "PASS"}, nil
}

func (s searchConsoleClientStub) QuerySearchAnalytics(context.Context, string, SearchRequest) ([]SearchRow, error) {
	return nil, nil
}
func (s searchConsoleClientStub) ListSites(context.Context) ([]Site, error) { return s.sites, s.err }
func (s searchConsoleClientStub) UserInfoEmail(context.Context) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.email, nil
}

func TestConnectionStatusReportsProjectAndUserState(t *testing.T) {
	storage := &connectionOpsStub{
		connection: ProjectProperty{SiteURL: "https://example.test/", ConnectedAccountEmail: stringPointer("owner@example.test"), CreatedAt: "created"},
		hasGrant:   true, canManage: false,
	}
	ops := &ConnectionOperations{Connections: storage, Grants: storage, Manager: storage, OAuthReady: true}
	got, err := ops.GetStatus(context.Background(), "member", "org", "project")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Connected || !got.CurrentUserHasGrant || got.CanManage || !got.GoogleOAuthConfigured || got.SiteURL == nil || *got.SiteURL != storage.connection.SiteURL {
		t.Fatalf("status = %+v", got)
	}
}

func TestListSitesMarksSelectedPropertyAndGrantFailures(t *testing.T) {
	storage := &connectionOpsStub{
		connection: ProjectProperty{SiteURL: "https://selected.test/", GSCAccountID: stringPointer("grant-a")},
		grants:     []Grant{{AccountID: "grant-a"}, {AccountID: "grant-b"}},
	}
	ops := &ConnectionOperations{
		Connections: storage, Grants: storage,
		NewClient: func(_, accountID string) SearchConsoleClient {
			if accountID == "grant-b" {
				return searchConsoleClientStub{err: errors.New("provider unavailable")}
			}
			return searchConsoleClientStub{email: "a@example.test", sites: []Site{
				{SiteURL: "https://selected.test/", PermissionLevel: "siteOwner"},
				{SiteURL: "https://unverified.test/", PermissionLevel: "siteUnverifiedUser"},
			}}
		},
	}
	got, err := ops.ListSites(context.Background(), "user", "org", "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Sites[0].IsSelected || !got[0].Sites[0].Selectable || got[0].Sites[1].Selectable || !got[1].PropertiesUnavailable {
		t.Fatalf("accounts = %+v", got)
	}
}

func TestSetSiteValidatesVerifiedGrantBeforeUpsert(t *testing.T) {
	storage := &connectionOpsStub{grants: []Grant{{AccountID: "grant-a"}}, canManage: true}
	ops := &ConnectionOperations{
		Grants: storage, Manager: storage,
		NewClient: func(_, _ string) SearchConsoleClient {
			return searchConsoleClientStub{sites: []Site{{SiteURL: "https://example.test/", PermissionLevel: "siteUnverifiedUser"}}}
		},
	}
	_, err := ops.SetSite(context.Background(), "user", "org", "project", "grant-a", "https://example.test/")
	if !errors.Is(err, ErrSiteUnverified) {
		t.Fatalf("error = %v, want ErrSiteUnverified", err)
	}
	if storage.upserted.SiteURL != "" {
		t.Fatal("unverified property was persisted")
	}

	storage.canManage = false
	_, err = ops.SetSite(context.Background(), "user", "org", "project", "grant-a", "https://example.test/")
	if !errors.Is(err, ErrManageForbidden) {
		t.Fatalf("error = %v, want ErrManageForbidden", err)
	}
}

func TestDisconnectRequiresManagerRole(t *testing.T) {
	storage := &connectionOpsStub{canManage: false}
	ops := &ConnectionOperations{Manager: storage}
	if err := ops.Disconnect(context.Background(), "user", "org", "project"); !errors.Is(err, ErrManageForbidden) {
		t.Fatalf("error = %v, want ErrManageForbidden", err)
	}
	if storage.deleted {
		t.Fatal("member disconnected the integration")
	}
}

func TestInspectURLsKeepsIndividualFailuresAndSelectedGrantScope(t *testing.T) {
	storage := &connectionOpsStub{connection: ProjectProperty{
		SiteURL: "https://example.test/", ConnectedByUserID: "connector", GSCAccountID: stringPointer("grant-a"),
		ConnectedAccountEmail: stringPointer("owner@example.test"),
	}}
	client := &inspectingConsoleClient{err: map[string]error{"https://example.test/bad": errors.New("provider error")}}
	ops := &ConnectionOperations{Connections: storage, NewClient: func(userID, accountID string) SearchConsoleClient {
		if userID != "connector" || accountID != "grant-a" {
			t.Fatalf("client scope = %q/%q", userID, accountID)
		}
		return client
	}}
	got, err := ops.InspectURLs(context.Background(), "org", "project", []string{"https://example.test/good", "https://example.test/bad"}, "en-US")
	if err != nil {
		t.Fatal(err)
	}
	results := got["results"].([]map[string]any)
	if len(results) != 2 || results[0]["result"] == nil || results[1]["result"] != nil || results[1]["error"] != "Inspection failed" {
		t.Fatalf("results = %#v", results)
	}
	if len(client.seen) != 2 {
		t.Fatalf("inspected URLs = %#v", client.seen)
	}
}
