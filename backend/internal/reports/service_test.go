package reports

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	items        map[string]Metadata
	html         map[string]string
	templates    map[string]Template
	bytes        int64
	rows         []Template
	shareCreated bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{items: map[string]Metadata{}, html: map[string]string{}, templates: map[string]Template{}, shareCreated: true}
}
func (m *memoryStore) List(_ context.Context, project string, limit, offset int) ([]Metadata, error) {
	out := []Metadata{}
	for _, v := range m.items {
		if v.ProjectID == project {
			out = append(out, v)
		}
	}
	return out, nil
}
func (m *memoryStore) Count(_ context.Context, project string) (int, error) {
	n := 0
	for _, v := range m.items {
		if v.ProjectID == project {
			n++
		}
	}
	return n, nil
}
func (m *memoryStore) Get(_ context.Context, project, id string) (Metadata, error) {
	v, ok := m.items[id]
	if !ok || v.ProjectID != project {
		return Metadata{}, ErrNotFound
	}
	return v, nil
}
func (m *memoryStore) GetHTML(ctx context.Context, project, id string) (Metadata, string, error) {
	v, e := m.Get(ctx, project, id)
	if e != nil {
		return Metadata{}, "", e
	}
	return v, m.html[id], nil
}
func (m *memoryStore) FindTitle(_ context.Context, project, title string) (string, error) {
	for id, v := range m.items {
		if v.ProjectID == project && v.Title == title {
			return id, nil
		}
	}
	return "", nil
}
func (m *memoryStore) OrganizationBytes(context.Context, string) (int64, error) { return m.bytes, nil }
func (m *memoryStore) Insert(_ context.Context, v Metadata, h string) error {
	m.items[v.ID] = v
	m.html[v.ID] = h
	m.bytes += int64(v.SizeBytes)
	return nil
}
func (m *memoryStore) Update(_ context.Context, v Metadata, h string) error {
	m.bytes += int64(v.SizeBytes - m.items[v.ID].SizeBytes)
	m.items[v.ID] = v
	m.html[v.ID] = h
	return nil
}
func (m *memoryStore) Delete(_ context.Context, project, id string) (bool, error) {
	v, e := m.Get(context.Background(), project, id)
	if e != nil {
		return false, nil
	}
	delete(m.items, id)
	delete(m.html, id)
	m.bytes -= int64(v.SizeBytes)
	return true, nil
}
func (m *memoryStore) SetShare(_ context.Context, project, id string, token, at *string) (bool, error) {
	v, ok := m.items[id]
	if !ok || v.ProjectID != project {
		return false, nil
	}
	if token != nil && v.ShareToken != nil {
		return false, nil
	}
	v.ShareToken = token
	v.SharedAt = at
	m.items[id] = v
	return m.shareCreated, nil
}
func (m *memoryStore) GetShared(_ context.Context, token string) (SharedReport, error) {
	for _, v := range m.items {
		if v.ShareToken != nil && *v.ShareToken == token {
			return SharedReport{Metadata: v, HTML: m.html[v.ID]}, nil
		}
	}
	return SharedReport{}, ErrNotFound
}
func (m *memoryStore) ListTemplates(context.Context, string) ([]Template, error) { return m.rows, nil }
func (m *memoryStore) GetTemplate(_ context.Context, project, id string) (Template, error) {
	v, ok := m.templates[id]
	if !ok || v.ProjectID != project {
		return Template{}, ErrNotFound
	}
	return v, nil
}
func (m *memoryStore) InsertTemplate(_ context.Context, v Template) error {
	m.templates[v.ID] = v
	m.rows = append(m.rows, v)
	return nil
}
func (m *memoryStore) UpdateTemplate(_ context.Context, v Template) (bool, error) {
	if _, ok := m.templates[v.ID]; !ok {
		return false, nil
	}
	m.templates[v.ID] = v
	return true, nil
}
func (m *memoryStore) DeleteTemplate(_ context.Context, project, id string) (bool, error) {
	v, ok := m.templates[id]
	if !ok || v.ProjectID != project {
		return false, nil
	}
	delete(m.templates, id)
	return true, nil
}

func TestSaveCreatesAndRejectsIncompleteOrDuplicateReport(t *testing.T) {
	store := newMemoryStore()
	svc := &Service{Store: store, Now: func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }}
	in := SaveInput{ProjectID: "p1", OrganizationID: "o1", Title: "Audit 2026-10-10", Summary: "Verdict and action", HTML: "<html><body>ok</body></html>", CreatedBy: "Tester", CreatedByUserID: "u1"}
	got, err := svc.Save(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created || got.HTMLBytes != len(in.HTML) || store.html[got.ReportID] != in.HTML {
		t.Fatalf("save = %#v, stored HTML %q", got, store.html[got.ReportID])
	}
	in.ReportID = ""
	if _, err = svc.Save(context.Background(), in); err == nil {
		t.Fatal("duplicate title unexpectedly accepted")
	}
	in.Title = "Other"
	in.HTML = "<html>partial"
	if _, err = svc.Save(context.Background(), in); err == nil {
		t.Fatal("incomplete HTML unexpectedly accepted")
	}
}

func TestSavePreservesSharingOnReplaceAndCountsUTF16Characters(t *testing.T) {
	store := newMemoryStore()
	svc := &Service{Store: store, Now: func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }}
	in := SaveInput{ProjectID: "p1", OrganizationID: "o1", Title: "Original", Summary: "Summary", HTML: "<html>one</html>"}
	created, err := svc.Save(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	token := "0123456789abcdefghijklmnopqrstuv"
	sharedAt := "2026-10-10T11:00:00.000Z"
	item := store.items[created.ReportID]
	item.ShareToken = &token
	item.SharedAt = &sharedAt
	store.items[created.ReportID] = item
	in.ReportID = created.ReportID
	in.HTML = "<html>two</html>"
	updated, err := svc.Save(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Created || store.items[created.ReportID].ShareToken == nil || *store.items[created.ReportID].ShareToken != token {
		t.Fatal("replace did not preserve existing share")
	}
	in.Title = strings.Repeat("??", 61)
	if _, err = svc.Save(context.Background(), in); err == nil {
		t.Fatal("UTF-16 title limit not enforced")
	}
}

func TestShareRequiresHostedAndCreatesOpaqueToken(t *testing.T) {
	store := newMemoryStore()
	svc := &Service{Store: store, NewShareToken: func() (string, error) { return "0123456789abcdefghijklmnopqrstuv", nil }}
	store.items["r1"] = Metadata{ID: "r1", ProjectID: "p1"}
	if _, err := svc.Share(context.Background(), "p1", "r1"); err == nil {
		t.Fatal("self-hosted share unexpectedly enabled")
	}
	svc.Hosted = true
	got, err := svc.Share(context.Background(), "p1", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ShareToken == nil || !validShareToken(*got.ShareToken) {
		t.Fatalf("share token = %#v", got.ShareToken)
	}
	if _, err := svc.GetShared(context.Background(), "short"); err == nil {
		t.Fatal("malformed public token accepted")
	}
}

func TestSaveWrapsStoreNotFoundAndPropagatesErrors(t *testing.T) {
	store := newMemoryStore()
	svc := &Service{Store: store}
	_, err := svc.Get(context.Background(), "p1", "missing")
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Code != "NOT_FOUND" {
		t.Fatalf("get error = %v", err)
	}
}
