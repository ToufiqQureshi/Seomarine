package projects

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	rows     []Project
	role     string
	archived bool
	restored bool
	updated  Input
}

func (f *fakeStore) List(context.Context, string, bool) ([]Project, error) { return f.rows, nil }
func (f *fakeStore) Get(_ context.Context, _, id string, _ bool) (Project, error) {
	for _, p := range f.rows {
		if p.ID == id {
			return p, nil
		}
	}
	return Project{}, ErrNotFound
}
func (f *fakeStore) Create(_ context.Context, _ string, in Input) (Project, error) {
	p := Project{ID: "new", Name: in.Name, Domain: in.Domain, LocationCode: marketDefault(), LanguageCode: "en"}
	f.rows = append(f.rows, p)
	return p, nil
}
func (f *fakeStore) Update(_ context.Context, _, id string, in Input) (Project, error) {
	f.updated = in
	return Project{ID: id, Name: in.Name, Domain: in.Domain}, nil
}
func (f *fakeStore) Archive(context.Context, string, string, string) error {
	f.archived = true
	return nil
}
func (f *fakeStore) Restore(context.Context, string, string) error { f.restored = true; return nil }
func (f *fakeStore) Count(context.Context, string) (int, error)    { return len(f.rows), nil }
func (f *fakeStore) EnsureDefault(context.Context, string) error {
	f.rows = []Project{{ID: "default", Name: "Default"}}
	return nil
}
func (f *fakeStore) Role(context.Context, string, string) (string, error) { return f.role, nil }
func marketDefault() int                                                  { return 2840 }

func TestProjectServiceEnsuresOneProjectAndValidatesOwnerOperations(t *testing.T) {
	store := &fakeStore{role: "member"}
	svc := &Service{Store: store, Now: func() time.Time { return time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC) }}
	rows, err := svc.List(context.Background(), "org", false, true)
	if err != nil || len(rows) != 1 || rows[0].Name != "Default" {
		t.Fatalf("default list=%+v err=%v", rows, err)
	}
	if _, err = svc.Create(context.Background(), "org", "u", Input{Name: "Site"}); err == nil {
		t.Fatal("member unexpectedly created project")
	}
	store.role = "owner"
	domain := "HTTPS://WWW.Example.com/path"
	created, err := svc.Create(context.Background(), "org", "u", Input{Name: " Site ", Domain: &domain})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Site" || created.Domain == nil || *created.Domain != "example.com" {
		t.Fatalf("created=%+v", created)
	}
}

func TestProjectServiceArchiveAndRestore(t *testing.T) {
	store := &fakeStore{role: "admin", rows: []Project{{ID: "a"}, {ID: "b"}}}
	svc := &Service{Store: store, Now: func() time.Time { return time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC) }}
	if err := svc.Archive(context.Background(), "org", "u", "a"); err != nil {
		t.Fatal(err)
	}
	if !store.archived {
		t.Fatal("archive not delegated")
	}
	if err := svc.Restore(context.Background(), "org", "u", "a"); err != nil {
		t.Fatal(err)
	}
	if !store.restored {
		t.Fatal("restore not delegated")
	}
	store.rows = store.rows[:1]
	if err := svc.Archive(context.Background(), "org", "u", "a"); err == nil {
		t.Fatal("only active project was archived")
	}
}

func TestNormalizeProjectDomainAndMarket(t *testing.T) {
	domain := "https://www.example.com/path"
	loc := 2840
	lang := "es"
	in := Input{Name: "A", Domain: &domain, LocationCode: &loc, LanguageCode: &lang}
	if err := normalize(&in); err != nil {
		t.Fatal(err)
	}
	if *in.Domain != "example.com" {
		t.Fatalf("domain=%q", *in.Domain)
	}
	bad := Input{Name: "A", LanguageCode: &lang}
	if err := normalize(&bad); err == nil {
		t.Fatal("language without location accepted")
	}
	bad = Input{Name: "A", LocationCode: &loc, LanguageCode: ptr("unsupported")}
	if err := normalize(&bad); err == nil {
		t.Fatal("unsupported market accepted")
	}
}
func ptr(v string) *string { return &v }
