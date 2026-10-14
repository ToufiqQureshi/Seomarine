package contenteditor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/uplumo/backend/internal/config"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/ids"
)

const (
	// MaxImageUploadSize is the server-side cap. The browser compresses images
	// to WebP first, so normal editor uploads are much smaller than this limit.
	MaxImageUploadSize = 1 << 20
	maxImageDimension  = 2_400
	maxImagePixels     = 8_000_000
)

// ErrImageStorageUnavailable indicates that Supabase image storage is absent.
var ErrImageStorageUnavailable = errors.New("content image storage is not configured")

// ImageValidationError indicates the uploaded file is not a supported image.
type ImageValidationError struct{ Message string }

func (e *ImageValidationError) Error() string { return e.Message }

// ImageStore validates and stores uploaded editor images, returning their
// stable public URL.
type ImageStore interface {
	Upload(ctx context.Context, data []byte) (string, error)
}

// ImageUploadLimiter caps storage uploads per organization across server
// instances. Implementations should fail closed because uploads consume paid
// storage.
type ImageUploadLimiter interface {
	Allow(ctx context.Context, organizationID string) (bool, error)
}

// RedisImageUploadLimiter limits image uploads to 20 per organization per
// hour, shared across application instances.
type RedisImageUploadLimiter struct{ Client *redis.Client }

// Allow reports whether the organization can upload another image this hour.
func (l RedisImageUploadLimiter) Allow(ctx context.Context, organizationID string) (bool, error) {
	if l.Client == nil {
		return false, errors.New("image upload limiter is not configured")
	}
	window := time.Now().UTC().Format("2006010215")
	key := "content-editor:image-upload:" + organizationID + ":" + window
	count, err := l.Client.Eval(ctx, `local count = redis.call('INCR', KEYS[1]); if count == 1 then redis.call('EXPIRE', KEYS[1], 7200) end; return count`, []string{key}).Int64()
	if err != nil {
		return false, fmt.Errorf("limit content image uploads: %w", err)
	}
	return count <= 20, nil
}

// SupabaseImageStore stores optimized raster images in a public Supabase
// Storage bucket. The service-role key is server-side only.
type SupabaseImageStore struct {
	client *http.Client
	url    *url.URL
	key    string
	bucket string
}

// NewSupabaseImageStore creates a server-side client for the configured bucket.
func NewSupabaseImageStore(cfg *config.SupabaseStorage) (*SupabaseImageStore, error) {
	if cfg == nil {
		return nil, ErrImageStorageUnavailable
	}
	return &SupabaseImageStore{
		client: &http.Client{Timeout: 30 * time.Second},
		url:    cfg.URL,
		key:    cfg.ServiceRoleKey,
		bucket: cfg.ContentImageBucket,
	}, nil
}

// Upload validates the actual image signature and dimensions before storage.
// It permits only WebP files produced by the editor's client-side optimizer.
func (s *SupabaseImageStore) Upload(ctx context.Context, data []byte) (string, error) {
	if s == nil || s.client == nil || s.url == nil || s.key == "" || s.bucket == "" {
		return "", ErrImageStorageUnavailable
	}
	if len(data) == 0 || len(data) > MaxImageUploadSize {
		return "", &ImageValidationError{Message: "Image must be between 1 byte and 1 MB after compression."}
	}
	width, height, ok := webpDimensions(data)
	if !ok {
		return "", &ImageValidationError{Message: "Choose a valid PNG or JPEG image. The editor converts uploads to WebP."}
	}
	if width < 1 || height < 1 || width > maxImageDimension || height > maxImageDimension || int64(width)*int64(height) > maxImagePixels {
		return "", &ImageValidationError{Message: "Image dimensions are too large; maximum is 2,400 pixels per side and 8 megapixels total."}
	}
	objectID, err := ids.New()
	if err != nil {
		return "", fmt.Errorf("create image object ID: %w", err)
	}
	key := "content-editor/" + objectID + ".webp"
	endpoint := *s.url
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/storage/v1/object/" + url.PathEscape(s.bucket) + "/" + key
	// #nosec G704 -- endpoint is derived from the HTTPS project origin validated at startup.
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create Supabase image upload request: %w", err)
	}
	request.Header.Set("apikey", s.key)
	request.Header.Set("Authorization", "Bearer "+s.key)
	request.Header.Set("Content-Type", "image/webp")
	request.Header.Set("Cache-Control", "public, max-age=31536000, immutable")
	request.Header.Set("x-upsert", "false")
	// #nosec G704 -- the request target is the configured Supabase origin, never user input.
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("upload content image to Supabase: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return "", fmt.Errorf("supabase image upload returned status %d", response.StatusCode)
	}
	publicURL := *s.url
	publicURL.Path = strings.TrimRight(publicURL.Path, "/") + "/storage/v1/object/public/" + url.PathEscape(s.bucket) + "/" + key
	return publicURL.String(), nil
}

// webpDimensions reads the WebP container dimensions without allocating a
// decoded bitmap. The parser accepts VP8, VP8L and VP8X headers.
func webpDimensions(data []byte) (width, height int, ok bool) {
	if len(data) < 30 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0, false
	}
	switch string(data[12:16]) {
	case "VP8X":
		if len(data) < 30 {
			return 0, 0, false
		}
		width = 1 + int(data[24]) + int(data[25])<<8 + int(data[26])<<16
		height = 1 + int(data[27]) + int(data[28])<<8 + int(data[29])<<16
	case "VP8L":
		if len(data) < 25 || data[20] != 0x2f {
			return 0, 0, false
		}
		width = 1 + int(data[21]) + (int(data[22])&0x3f)<<8
		height = 1 + (int(data[22]) >> 6) + int(data[23])<<2 + (int(data[24])&0x0f)<<10
	case "VP8 ":
		if len(data) < 30 || data[23] != 0x9d || data[24] != 0x01 || data[25] != 0x2a {
			return 0, 0, false
		}
		width = int(data[26]) | int(data[27]&0x3f)<<8
		height = int(data[28]) | int(data[29]&0x3f)<<8
	default:
		return 0, 0, false
	}
	return width, height, width > 0 && height > 0
}

// readUpload reads at most limit+1 bytes so oversized files are rejected
// without allowing an unbounded multipart body into memory.
func readUpload(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("image is too large; maximum upload size is 1 MB")
	}
	return data, nil
}
