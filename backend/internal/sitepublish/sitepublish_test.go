package sitepublish

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/contenteditor"
	"github.com/toufiqqureshi/uplumo/backend/internal/google"
)

// memoryStore keeps connections and publications in memory for service
// tests; Repository is tested against Postgres.
type memoryStore struct {
	mu           sync.Mutex
	connections  map[string]Connection
	publications map[string]Publication
}

func newMemoryStore() *memoryStore {
	return &memoryStore{connections: map[string]Connection{}, publications: map[string]Publication{}}
}

func (m *memoryStore) Connection(_ context.Context, projectID string) (Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.connections[projectID]
	if !ok {
		return Connection{}, ErrNotConnected
	}
	return c, nil
}

func (m *memoryStore) SaveConnection(_ context.Context, c Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[c.ProjectID] = c
	return nil
}

func (m *memoryStore) DeleteConnection(_ context.Context, projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.connections, projectID)
	return nil
}

func (m *memoryStore) Publication(_ context.Context, projectID, draftID string) (Publication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.publications[draftID]
	if !ok || p.ProjectID != projectID {
		return Publication{}, ErrNotFound
	}
	return p, nil
}

func (m *memoryStore) SavePublication(_ context.Context, p Publication) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publications[p.DraftID] = p
	return nil
}

type memoryDrafts map[string]contenteditor.Draft

func (d memoryDrafts) Get(_ context.Context, projectID, id string) (contenteditor.Draft, error) {
	draft, ok := d[id]
	if !ok || draft.ProjectID != projectID {
		return contenteditor.Draft{}, contenteditor.ErrNotFound
	}
	return draft, nil
}

func testCipher(t *testing.T) *google.TokenCipher {
	t.Helper()
	cipher, err := google.NewTokenCipher("test", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func newTestService(t *testing.T, wp *fakeWordPress, drafts memoryDrafts) (*Service, *memoryStore) {
	t.Helper()
	store := newMemoryStore()
	return &Service{Store: store, Drafts: drafts, Cipher: testCipher(t), WordPress: wp.client()}, store
}

func connect(t *testing.T, service *Service, wp *fakeWordPress, projectID string) {
	t.Helper()
	if _, err := service.Connect(t.Context(), projectID, "user-1", ConnectInput{
		Platform: WordPressPlatform, SiteURL: wp.server.URL, Username: testUser, ApplicationPassword: testPassword,
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
}

func TestConnectStoresTheEncryptedPasswordOnlyAfterWordPressAcceptsIt(t *testing.T) {
	wp := newFakeWordPress(t)
	service, store := newTestService(t, wp, nil)

	_, err := service.Connect(t.Context(), "project-1", "user-1", ConnectInput{Platform: WordPressPlatform, SiteURL: wp.server.URL, Username: testUser, ApplicationPassword: "wrong password"})
	if !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("wrong password: err = %v, want ErrBadCredentials", err)
	}
	if _, err := store.Connection(t.Context(), "project-1"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("a refused password was stored: %v", err)
	}

	// WordPress shows application passwords as "abcd EFGH ijkl ...".
	spaced := strings.Join([]string{testPassword[0:4], testPassword[4:8], testPassword[8:12], testPassword[12:16], testPassword[16:20], testPassword[20:24]}, " ")
	connection, err := service.Connect(t.Context(), "project-1", "user-1", ConnectInput{Platform: WordPressPlatform, SiteURL: " " + wp.server.URL + "/ ", Username: " " + testUser + " ", ApplicationPassword: spaced})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if connection.SiteURL != wp.server.URL || connection.Username != testUser || connection.SiteName != "Test Blog" || connection.ConnectedBy != "user-1" {
		t.Errorf("connection = %+v", connection)
	}
	if strings.Contains(connection.SecretCiphertext, testPassword) {
		t.Fatal("the password is stored in plain text")
	}
	if plain, err := testCipher(t).Decrypt(connection.SecretCiphertext); err != nil || plain != testPassword {
		t.Errorf("stored secret decrypts to %q, err %v", plain, err)
	}
}

func TestConnectRejectsBadInputBeforeCallingTheSite(t *testing.T) {
	wp := newFakeWordPress(t)
	service, _ := newTestService(t, wp, nil)
	cases := []ConnectInput{
		{Platform: "shopify", SiteURL: wp.server.URL, Username: testUser, ApplicationPassword: testPassword},
		{Platform: WordPressPlatform, SiteURL: "", Username: testUser, ApplicationPassword: testPassword},
		{Platform: WordPressPlatform, SiteURL: "https://someone@example.com", Username: testUser, ApplicationPassword: testPassword},
		{Platform: WordPressPlatform, SiteURL: wp.server.URL, Username: "", ApplicationPassword: testPassword},
		{Platform: WordPressPlatform, SiteURL: wp.server.URL, Username: testUser, ApplicationPassword: "   "},
	}
	for _, input := range cases {
		var validation *ValidationError
		if _, err := service.Connect(t.Context(), "p", "u", input); !errors.As(err, &validation) {
			t.Errorf("%+v: err = %v, want a ValidationError", input, err)
		}
	}
	if _, err := service.Connect(t.Context(), "p", "u", ConnectInput{Platform: WordPressPlatform, SiteURL: "http://example.com", Username: testUser, ApplicationPassword: testPassword}); !errors.Is(err, ErrNotHTTPS) {
		t.Errorf("http site: err = %v, want ErrNotHTTPS", err)
	}
	if _, _, requests := wp.snapshot(); len(requests) != 0 {
		t.Errorf("the site was called for invalid input: %v", requests)
	}
}

func TestPublishCopiesImagesOnceAndUpdatesTheSamePost(t *testing.T) {
	wp := newFakeWordPress(t)
	images := newImageHost(t)
	drafts := memoryDrafts{"draft-1": {
		ID: "draft-1", ProjectID: "project-1", Title: " Best SEO tools ", Slug: "best-seo-tools", MetaDescription: "A short list.",
		Content: fmt.Sprintf("# Intro\n\n![Chart](%[1]s/a.png)\n\nText with ![again](%[1]s/a.png) and ![second](%[1]s/b.png).\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))", images.URL),
	}}
	service, _ := newTestService(t, wp, drafts)
	connect(t, service, wp, "project-1")

	first, _, err := service.Publish(t.Context(), "project-1", "draft-1", StatusDraft, SchemaOptions{Article: true, FAQ: true})
	if err != nil {
		t.Fatalf("first Publish: %v", err)
	}
	posts, media, _ := wp.snapshot()
	if len(posts) != 1 || len(media) != 2 {
		t.Fatalf("after first publish: %d posts, %d images uploaded, want 1 and 2", len(posts), len(media))
	}
	var post Post
	for _, p := range posts {
		post = p
	}
	if post.Title != "Best SEO tools" || post.Slug != "best-seo-tools" || post.Excerpt != "A short list." || post.Status != StatusDraft {
		t.Errorf("post = %+v", post)
	}
	if strings.Contains(post.Content, images.URL) || !strings.Contains(post.Content, wp.server.URL+"/uploads/") {
		t.Errorf("images still point at the draft's host: %s", post.Content)
	}
	if strings.Contains(post.Content, "<script>") || strings.Contains(post.Content, "javascript:") {
		t.Errorf("unsafe HTML reached the site: %s", post.Content)
	}
	if post.FeaturedMedia == 0 || post.FeaturedMedia != first.Media[images.URL+"/a.png"].ID {
		t.Errorf("featured image = %d, want the first image %d", post.FeaturedMedia, first.Media[images.URL+"/a.png"].ID)
	}

	second, _, err := service.Publish(t.Context(), "project-1", "draft-1", StatusPublish, SchemaOptions{Article: true, FAQ: true})
	if err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	posts, media, _ = wp.snapshot()
	if len(posts) != 1 || len(media) != 2 {
		t.Errorf("after publishing again: %d posts, %d uploads; want the same post updated and no new uploads", len(posts), len(media))
	}
	if second.RemoteID != first.RemoteID || second.RemoteStatus != StatusPublish {
		t.Errorf("second publish = %+v, want post %s now published", second, first.RemoteID)
	}
}

func TestPublishRecreatesAPostDeletedOnTheSite(t *testing.T) {
	wp := newFakeWordPress(t)
	drafts := memoryDrafts{"d": {ID: "d", ProjectID: "p", Title: "T", Content: "Body"}}
	service, _ := newTestService(t, wp, drafts)
	connect(t, service, wp, "p")
	first, _, err := service.Publish(t.Context(), "p", "d", StatusDraft, SchemaOptions{Article: true, FAQ: true})
	if err != nil {
		t.Fatal(err)
	}
	wp.deletePosts()
	second, _, err := service.Publish(t.Context(), "p", "d", StatusDraft, SchemaOptions{Article: true, FAQ: true})
	if err != nil {
		t.Fatalf("Publish after the post was deleted: %v", err)
	}
	if posts, _, _ := wp.snapshot(); len(posts) != 1 || second.RemoteID == first.RemoteID {
		t.Errorf("posts = %d, ids %s -> %s; want one new post", len(posts), first.RemoteID, second.RemoteID)
	}
}

func TestPublishStartsFreshAfterConnectingAnotherSite(t *testing.T) {
	images := newImageHost(t)
	drafts := memoryDrafts{"d": {ID: "d", ProjectID: "p", Title: "T", Content: "![a](" + images.URL + "/a.png)"}}
	oldSite := newFakeWordPress(t)
	service, _ := newTestService(t, oldSite, drafts)
	connect(t, service, oldSite, "p")
	if _, _, err := service.Publish(t.Context(), "p", "d", StatusDraft, SchemaOptions{Article: true, FAQ: true}); err != nil {
		t.Fatal(err)
	}
	newSite := newFakeWordPress(t)
	service.WordPress = newSite.client()
	connect(t, service, newSite, "p")
	if _, _, err := service.Publish(t.Context(), "p", "d", StatusDraft, SchemaOptions{Article: true, FAQ: true}); err != nil {
		t.Fatalf("Publish to the new site: %v", err)
	}
	posts, media, _ := newSite.snapshot()
	if len(posts) != 1 || len(media) != 1 {
		t.Errorf("new site got %d posts and %d images, want 1 and 1", len(posts), len(media))
	}
}

func TestPublishRefusals(t *testing.T) {
	wp := newFakeWordPress(t)
	images := newImageHost(t)
	var manyImages strings.Builder
	for i := range MaxImagesPerArticle + 1 {
		fmt.Fprintf(&manyImages, "![x](%s/a.png?n=%d)\n", images.URL, i)
	}
	drafts := memoryDrafts{
		"empty":   {ID: "empty", ProjectID: "p", Title: "Title", Content: "  "},
		"many":    {ID: "many", ProjectID: "p", Title: "Title", Content: manyImages.String()},
		"broken":  {ID: "broken", ProjectID: "p", Title: "Title", Content: "![x](" + images.URL + "/missing.png)"},
		"other":   {ID: "other", ProjectID: "elsewhere", Title: "Title", Content: "Body"},
		"fine":    {ID: "fine", ProjectID: "p", Title: "Title", Content: "Body"},
		"svgOnly": {ID: "svgOnly", ProjectID: "p", Title: "Title", Content: "![x](" + images.URL + "/logo.svg)"},
	}
	service, _ := newTestService(t, wp, drafts)

	if _, _, err := service.Publish(t.Context(), "p", "fine", StatusDraft, SchemaOptions{Article: true, FAQ: true}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("not connected: err = %v", err)
	}
	connect(t, service, wp, "p")
	for _, id := range []string{"empty", "many", "broken", "svgOnly"} {
		var validation *ValidationError
		if _, _, err := service.Publish(t.Context(), "p", id, StatusDraft, SchemaOptions{Article: true, FAQ: true}); !errors.As(err, &validation) {
			t.Errorf("%s: err = %v, want a ValidationError", id, err)
		}
	}
	if _, _, err := service.Publish(t.Context(), "p", "fine", "private", SchemaOptions{}); err == nil {
		t.Error("status private was accepted")
	}
	if _, _, err := service.Publish(t.Context(), "p", "other", StatusDraft, SchemaOptions{Article: true, FAQ: true}); !errors.Is(err, contenteditor.ErrNotFound) {
		t.Errorf("another project's draft: err = %v, want ErrNotFound", err)
	}
	if posts, media, _ := wp.snapshot(); len(posts) != 0 || len(media) != 0 {
		t.Errorf("a refused publish still wrote %d posts and %d images", len(posts), len(media))
	}
}

func TestNormalizeSiteURL(t *testing.T) {
	cases := map[string]string{
		"example.com":                 "https://example.com",
		"https://Example.com/":        "https://example.com",
		"https://example.com/blog/":   "https://example.com/blog",
		"  www.example.co.in  ":       "https://www.example.co.in",
		"https://example.com:8443/wp": "https://example.com:8443/wp",
	}
	for input, want := range cases {
		if got, err := normalizeSiteURL(input); err != nil || got != want {
			t.Errorf("normalizeSiteURL(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, bad := range []string{"", "https://", "https://example.com/?a=1", "https://example.com/#x", strings.Repeat("a", 301)} {
		if _, err := normalizeSiteURL(bad); err == nil {
			t.Errorf("normalizeSiteURL(%q) accepted", bad)
		}
	}
}
