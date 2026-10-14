package contenteditor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
)

const maxRequestBody = MaxContentLength + (16 << 10)
const maxImageRequestBody = MaxImageUploadSize + (64 << 10)

// Store defines persistence operations used by the content editor.
type Store interface {
	List(context.Context, string) ([]DraftSummary, error)
	Get(context.Context, string, string) (Draft, error)
	Save(context.Context, string, Input) (Draft, error)
	Delete(context.Context, string, string) error
}

// Handler serves content editor requests.
type Handler struct {
	Store        Store
	Writer       WriterService
	Images       ImageStore
	ImageLimiter ImageUploadLimiter
	Logger       *slog.Logger
}

// Mount registers content editor endpoints on mux.
func Mount(mux *http.ServeMux, store Store, writer WriterService, images ImageStore, imageLimiter ImageUploadLimiter, withSession, withProjectAccess func(http.Handler) http.Handler) {
	handler := Handler{Store: store, Writer: writer, Images: images, ImageLimiter: imageLimiter, Logger: slog.Default()}
	protect := func(next http.Handler) http.Handler { return withSession(withProjectAccess(next)) }
	mux.Handle("POST /api/v1/projects/{projectId}/content/images/upload", protect(http.HandlerFunc(handler.uploadImage)))
	mux.Handle("POST /api/v1/projects/{projectId}/content/drafts/list", protect(http.HandlerFunc(handler.list)))
	mux.Handle("POST /api/v1/projects/{projectId}/content/drafts/{draftId}/get", protect(http.HandlerFunc(handler.get)))
	mux.Handle("POST /api/v1/projects/{projectId}/content/drafts/save", protect(http.HandlerFunc(handler.save)))
	mux.Handle("POST /api/v1/projects/{projectId}/content/drafts/{draftId}/delete", protect(http.HandlerFunc(handler.delete)))
	mux.Handle("POST /api/v1/projects/{projectId}/content/assistant/generate", protect(http.HandlerFunc(handler.generate)))
}

func (h Handler) uploadImage(w http.ResponseWriter, r *http.Request) {
	if h.Images == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_image_storage_unavailable", "Image uploads are not configured on this server yet. You can still insert a public image URL.")
		return
	}
	organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || organizationID == "" {
		httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
		return
	}
	if h.ImageLimiter == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_image_storage_unavailable", "Image uploads are temporarily unavailable.")
		return
	}
	allowed, err := h.ImageLimiter.Allow(r.Context(), organizationID)
	if err != nil {
		h.internalError(w, r, "limit content image uploads", err)
		return
	}
	if !allowed {
		httpx.WriteError(w, http.StatusTooManyRequests, "image_upload_limit", "This workspace has reached its image upload limit. Try again in an hour.")
		return
	}
	if r.ContentLength > maxImageRequestBody {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Compressed images must be 1 MB or smaller.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImageRequestBody)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Compressed images must be 1 MB or smaller.")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "invalid_image_upload", "Choose one compressed WebP image up to 1 MB.")
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	if len(r.MultipartForm.Value) != 0 || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["image"]) != 1 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_image_upload", "Choose one compressed WebP image up to 1 MB.")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_image_upload", "Choose one compressed WebP image up to 1 MB.")
		return
	}
	defer func() { _ = file.Close() }()
	if header.Size > MaxImageUploadSize {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Compressed images must be 1 MB or smaller.")
		return
	}
	data, err := readUpload(file, MaxImageUploadSize)
	if err != nil {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Compressed images must be 1 MB or smaller.")
		return
	}
	imageURL, err := h.Images.Upload(r.Context(), data)
	if err != nil {
		var validationErr *ImageValidationError
		if errors.As(err, &validationErr) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_image", validationErr.Error())
			return
		}
		h.internalError(w, r, "upload content image", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"url": imageURL})
}

func (h Handler) generate(w http.ResponseWriter, r *http.Request) {
	if h.Writer == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_assistant_unavailable", "The writing assistant is not available on this server.")
		return
	}
	var input WriterInput
	if !decodeLimit(w, r, &input, 128<<10) {
		return
	}
	organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || organizationID == "" {
		httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
		return
	}
	output, err := h.Writer.Generate(r.Context(), organizationID, input)
	var validationError *ValidationError
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, output)
	case errors.Is(err, ErrWriterUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_assistant_unavailable", "The writing assistant is not available on this server.")
	case errors.Is(err, ErrWriterNoCredits):
		httpx.WriteError(w, http.StatusPaymentRequired, "insufficient_credits", "This organization has no AI credits. Add credits to use the writing assistant.")
	case errors.Is(err, ErrWriterPlanRequired):
		httpx.WriteError(w, http.StatusPaymentRequired, "plan_required", "AI writing is available on Pro and Agency plans.")
	case errors.Is(err, ErrWriterTimeout):
		httpx.WriteError(w, http.StatusGatewayTimeout, "content_assistant_timeout", "The writing assistant took too long to respond. Try again.")
	case errors.As(err, &validationError):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", validationError.Error())
	default:
		h.internalError(w, r, "generate content suggestion", err)
	}
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_editor_unavailable", "The content editor is not available on this server.")
		return
	}
	var empty map[string]json.RawMessage
	if !decode(w, r, &empty) {
		return
	}
	if len(empty) != 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid content draft request.")
		return
	}
	drafts, err := h.Store.List(r.Context(), r.PathValue("projectId"))
	if err != nil {
		h.internalError(w, r, "list content drafts", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, drafts)
}

func (h Handler) save(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_editor_unavailable", "The content editor is not available on this server.")
		return
	}
	var input Input
	if !decode(w, r, &input) {
		return
	}
	draft, err := h.Store.Save(r.Context(), r.PathValue("projectId"), input)
	var validationError *ValidationError
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, draft)
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "This project or draft is no longer available.")
	case errors.Is(err, ErrDraftLimit):
		httpx.WriteError(w, http.StatusConflict, "draft_limit", "This project has reached its 100 saved draft limit. Delete an old draft to make room.")
	case errors.As(err, &validationError):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", validationError.Error())
	default:
		h.internalError(w, r, "save content draft", err)
	}
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_editor_unavailable", "The content editor is not available on this server.")
		return
	}
	var empty map[string]json.RawMessage
	if !decode(w, r, &empty) {
		return
	}
	if len(empty) != 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid content draft request.")
		return
	}
	draft, err := h.Store.Get(r.Context(), r.PathValue("projectId"), r.PathValue("draftId"))
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "This draft is no longer available.")
		return
	}
	if err != nil {
		h.internalError(w, r, "get content draft", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, draft)
}

func (h Handler) delete(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "content_editor_unavailable", "The content editor is not available on this server.")
		return
	}
	var empty map[string]json.RawMessage
	if !decode(w, r, &empty) {
		return
	}
	if len(empty) != 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid content draft request.")
		return
	}
	err := h.Store.Delete(r.Context(), r.PathValue("projectId"), r.PathValue("draftId"))
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "This draft is no longer available.")
	default:
		h.internalError(w, r, "delete content draft", err)
	}
}

func (h Handler) internalError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if r.Context().Err() != nil {
		return
	}
	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.ErrorContext(r.Context(), operation, "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeLimit(w, r, dst, maxRequestBody)
}

func decodeLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	trimmed := bytes.TrimSpace(body)
	if err != nil || len(trimmed) == 0 || trimmed[0] != '{' {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid content draft request.")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid content draft request.")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
		return false
	}
	return true
}
