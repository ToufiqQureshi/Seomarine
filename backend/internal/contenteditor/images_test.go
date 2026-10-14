package contenteditor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/config"
)

func testWebP() []byte {
	data := make([]byte, 30)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	data[24] = 2
	data[27] = 1
	return data
}

func TestSupabaseImageStoreValidatesAndUploadsCompressedWebP(t *testing.T) {
	data := testWebP()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/storage/v1/object/content-images/content-editor/") || !strings.HasSuffix(r.URL.Path, ".webp") {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("apikey") != "service-key" || r.Header.Get("Authorization") != "Bearer service-key" {
			t.Errorf("Supabase auth headers are missing")
		}
		if r.Header.Get("Content-Type") != "image/webp" || r.Header.Get("x-upsert") != "false" {
			t.Errorf("upload headers = %v", r.Header)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, data) {
			t.Errorf("uploaded bytes = %v, err = %v", body, err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	baseURL, _ := url.Parse(server.URL)
	store := &SupabaseImageStore{
		client: server.Client(), url: baseURL, key: "service-key", bucket: "content-images",
	}
	got, err := store.Upload(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, server.URL+"/storage/v1/object/public/content-images/content-editor/") || !strings.HasSuffix(got, ".webp") {
		t.Fatalf("public URL = %q", got)
	}
	if _, err := store.Upload(context.Background(), []byte("<svg><script>alert(1)</script></svg>")); err == nil {
		t.Fatal("accepted SVG instead of an optimized WebP image")
	}
}

func TestSupabaseImageStoreRejectsOversizedAndInvalidWebP(t *testing.T) {
	store := &SupabaseImageStore{}
	if _, err := store.Upload(context.Background(), bytes.Repeat([]byte("x"), MaxImageUploadSize+1)); err == nil {
		t.Fatal("accepted an oversized image")
	}
	if _, err := store.Upload(context.Background(), []byte("not a webp")); err == nil {
		t.Fatal("accepted invalid WebP data")
	}
}

func TestNewSupabaseImageStoreRequiresConfiguration(t *testing.T) {
	if _, err := NewSupabaseImageStore(nil); !errors.Is(err, ErrImageStorageUnavailable) {
		t.Fatalf("NewSupabaseImageStore(nil) error = %v", err)
	}
	projectURL, _ := url.Parse("https://project.supabase.co")
	if _, err := NewSupabaseImageStore(&config.SupabaseStorage{URL: projectURL, ServiceRoleKey: "secret", ContentImageBucket: "images"}); err != nil {
		t.Fatal(err)
	}
}

func TestReadUploadRejectsOversizedInput(t *testing.T) {
	if _, err := readUpload(bytes.NewReader([]byte("12345")), 4); err == nil {
		t.Fatal("readUpload accepted input beyond the limit")
	}
	got, err := readUpload(bytes.NewReader([]byte("1234")), 4)
	if err != nil || string(got) != "1234" {
		t.Fatalf("readUpload = %q, %v", got, err)
	}
}
