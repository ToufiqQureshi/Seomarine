package gsc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxPerformanceBody = 64 << 10

// PerformanceDeps supplies report service dependencies and access middleware.
type PerformanceDeps struct {
	Logger            *slog.Logger
	Service           *Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// MountPerformance registers the Search Performance report and table endpoints.
func MountPerformance(mux *http.ServeMux, d PerformanceDeps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	base := "POST /api/v1/projects/{projectId}/gsc/search-performance/"
	mux.Handle(base+"report", protect(performanceHandler(d, runPerformanceReport)))
	mux.Handle(base+"table", protect(performanceHandler(d, runPerformanceTable)))
	mux.Handle(base+"export", protect(performanceHandler(d, runPerformanceExport)))
}

func performanceHandler(d PerformanceDeps, call func(*Service, http.ResponseWriter, *http.Request, string, string) (any, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "gsc_unavailable", "Search Console reporting is not set up on this server.")
			return
		}
		if _, ok := auth.UserFromContext(r.Context()); !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
			return
		}
		organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
		if !ok {
			d.Logger.ErrorContext(r.Context(), "GSC report request without project organization")
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		result, err := call(d.Service, w, r, organizationID, r.PathValue("projectId"))
		if err != nil {
			writePerformanceError(d.Logger, w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})
}

func decodePerformance(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPerformanceBody))
	decoder.DisallowUnknownFields()
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil || len(raw) == 0 || raw[0] != '{' {
		return validationError("The request must be a JSON object.")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return validationError("The request must be a JSON object.")
	}
	for key, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return validationError("Request fields cannot be null.")
		}
		if key == "pageFilter" || key == "queryFilter" {
			var filterFields map[string]json.RawMessage
			if err := json.Unmarshal(value, &filterFields); err == nil {
				for _, filterValue := range filterFields {
					if bytes.Equal(bytes.TrimSpace(filterValue), []byte("null")) {
						return validationError("Filter fields cannot be null.")
					}
				}
			}
		}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return validationError("The request must contain exactly one JSON value.")
	}
	decoder = json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return validationError("The request is not valid JSON.")
	}
	return nil
}

func runPerformanceReport(s *Service, w http.ResponseWriter, r *http.Request, orgID, projectID string) (any, error) {
	var input PerformanceInput
	if err := decodePerformance(w, r, &input); err != nil {
		return nil, err
	}
	if err := validatePerformanceInput(&input); err != nil {
		return nil, err
	}
	return s.GetPerformance(r.Context(), orgID, projectID, input)
}
func runPerformanceTable(s *Service, w http.ResponseWriter, r *http.Request, orgID, projectID string) (any, error) {
	var input TableInput
	if err := decodePerformance(w, r, &input); err != nil {
		return nil, err
	}
	if err := validatePerformanceInput(&input.PerformanceInput); err != nil {
		return nil, err
	}
	if input.Page.Present && input.Page.Value < 1 || input.PageSize.Present && input.PageSize.Value < 1 {
		return nil, validationError("Page and pageSize must be positive values.")
	}
	return s.GetTable(r.Context(), orgID, projectID, input)
}
func runPerformanceExport(s *Service, w http.ResponseWriter, r *http.Request, orgID, projectID string) (any, error) {
	var input ExportInput
	if err := decodePerformance(w, r, &input); err != nil {
		return nil, err
	}
	if err := validatePerformanceInput(&input.PerformanceInput); err != nil {
		return nil, err
	}
	return s.ExportTable(r.Context(), orgID, projectID, input)
}

func validatePerformanceInput(input *PerformanceInput) error {
	if input.DateRange != "" && input.DateRange != last7Days && input.DateRange != last28Days && input.DateRange != last3Months {
		return validationError("dateRange must be last_7_days, last_28_days, or last_3_months.")
	}
	if input.Device != "" && input.Device != "DESKTOP" && input.Device != "MOBILE" && input.Device != "TABLET" {
		return validationError("device must be DESKTOP, MOBILE, or TABLET.")
	}
	if input.Country != "" {
		if len(input.Country) != 3 {
			return validationError("country must be a three-letter country code.")
		}
		for _, c := range input.Country {
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
				return validationError("country must be a three-letter country code.")
			}
		}
		input.Country = strings.ToLower(input.Country)
	}
	for _, filter := range []*TextFilter{input.PageFilter, input.QueryFilter} {
		if filter == nil {
			continue
		}
		filter.Expression = strings.TrimSpace(filter.Expression)
		if filter.Operator != "contains" && filter.Operator != "equals" {
			return validationError("filter operator must be contains or equals.")
		}
		length := len(utf16.Encode([]rune(filter.Expression)))
		if length == 0 || length > 4096 || len(filter.Expression) > 1024 {
			return validationError("filter expression must contain 1 to 4096 characters and fit the provider request limit.")
		}
	}
	if _, err := resolveDateRange(*input, time.Now().UTC()); err != nil {
		return validationError(err.Error())
	}
	return nil
}

func writePerformanceError(logger *slog.Logger, w http.ResponseWriter, r *http.Request, err error) {
	var invalid validationError
	if errors.As(err, &invalid) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", invalid.Error())
		return
	}
	if errors.Is(err, ErrConnectionNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "gsc_not_connected", "Search Console is not connected for this project.")
		return
	}
	var provider reportError
	if errors.As(err, &provider) {
		httpx.WriteError(w, provider.status, provider.code, provider.message)
		return
	}
	if r.Context().Err() != nil {
		logger.InfoContext(r.Context(), "GSC report request canceled", "err", err)
		return
	}
	logger.ErrorContext(r.Context(), "GSC report failed", "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}

// validationError marks invalid user-provided report parameters.
type validationError string

func (e validationError) Error() string { return string(e) }
