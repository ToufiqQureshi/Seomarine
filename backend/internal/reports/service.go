package reports

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

type Service struct {
	Store         Store
	Hosted        bool
	Now           func() time.Time
	NewShareToken func() (string, error)
}

type SaveInput struct {
	ProjectID, OrganizationID, ReportID, Title, Summary, HTML, CreatedBy, CreatedByUserID string
	Skill, TemplateID                                                                     *string
}
type SaveResult struct {
	ReportID  string `json:"reportId"`
	Title     string `json:"title"`
	Created   bool   `json:"created"`
	HTMLBytes int    `json:"htmlBytes"`
}
type ListResult struct {
	Reports    []Metadata `json:"reports"`
	TotalCount int        `json:"totalCount"`
	RowCount   int        `json:"rowCount"`
	Remaining  int        `json:"remaining"`
}
type ListTemplateResult struct {
	Templates []Template `json:"templates"`
	Remaining int        `json:"remaining"`
}

func (s *Service) Save(ctx context.Context, in SaveInput) (SaveResult, error) {
	if s == nil || s.Store == nil {
		return SaveResult{}, fail("reports_unavailable", "Reports are not available on this server.")
	}
	if in.ProjectID == "" || in.OrganizationID == "" || strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Summary) == "" || strings.TrimSpace(in.HTML) == "" {
		return SaveResult{}, fail("VALIDATION_ERROR", "projectId, title, summary, and complete HTML are required.")
	}
	if chars(in.Title) > MaxTitleCharacters {
		return SaveResult{}, fail("VALIDATION_ERROR", fmt.Sprintf("Title exceeds the %d character limit.", MaxTitleCharacters))
	}
	if chars(in.Summary) > MaxSummaryCharacters {
		return SaveResult{}, fail("VALIDATION_ERROR", fmt.Sprintf("Summary exceeds the %d character limit.", MaxSummaryCharacters))
	}
	if in.Skill != nil && chars(*in.Skill) > MaxSkillCharacters {
		return SaveResult{}, fail("VALIDATION_ERROR", fmt.Sprintf("Skill exceeds the %d character limit.", MaxSkillCharacters))
	}
	size := len([]byte(in.HTML))
	if size > MaxHTMLBytes {
		return SaveResult{}, fail("VALIDATION_ERROR", fmt.Sprintf("Report is %s; the limit is %s. Remove inlined images and save again.", kbUp(size), kbDown(MaxHTMLBytes)))
	}
	trimmed := strings.ToLower(strings.TrimSpace(in.HTML))
	if !strings.Contains(trimmed, "<html") || !strings.HasSuffix(trimmed, "</html>") {
		return SaveResult{}, fail("VALIDATION_ERROR", "The HTML is incomplete; it must end with </html>. Save was refused to preserve the existing report.")
	}
	if in.TemplateID != nil && *in.TemplateID != "" {
		if _, err := s.Store.GetTemplate(ctx, in.ProjectID, *in.TemplateID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return SaveResult{}, fail("NOT_FOUND", "Report template does not exist in this project.")
			}
			return SaveResult{}, fmt.Errorf("validate report template: %w", err)
		}
	}
	var existing Metadata
	if in.ReportID != "" {
		row, err := s.Store.Get(ctx, in.ProjectID, in.ReportID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return SaveResult{}, fail("NOT_FOUND", "Report does not exist in this project. List reports or omit reportId to create a new one.")
			}
			return SaveResult{}, fmt.Errorf("load report before save: %w", err)
		}
		existing = row
	}
	clash, err := s.Store.FindTitle(ctx, in.ProjectID, in.Title)
	if err != nil {
		return SaveResult{}, err
	}
	if clash != "" && clash != existing.ID {
		return SaveResult{}, fail("VALIDATION_ERROR", "A report with this title already exists in the project. Pass its reportId to replace it, or use another title.")
	}
	if existing.ID == "" {
		count, err := s.Store.Count(ctx, in.ProjectID)
		if err != nil {
			return SaveResult{}, err
		}
		if count >= MaxReportsPerProject {
			return SaveResult{}, fail("VALIDATION_ERROR", "This project has reached the report limit. Delete an older report first.")
		}
	}
	orgBytes, err := s.Store.OrganizationBytes(ctx, in.OrganizationID)
	if err != nil {
		return SaveResult{}, err
	}
	if orgBytes-int64(existing.SizeBytes)+int64(size) > MaxReportBytesPerOrg {
		return SaveResult{}, fail("VALIDATION_ERROR", "This organization has reached its report storage limit. Delete reports that are no longer needed.")
	}
	now := utcStamp(s.now())
	if existing.ID != "" {
		existing.Title = in.Title
		existing.Summary = in.Summary
		existing.SizeBytes = size
		existing.UpdatedAt = now
		if in.Skill != nil {
			existing.Skill = in.Skill
		}
		if in.TemplateID != nil {
			existing.TemplateID = in.TemplateID
		}
		if err := s.Store.Update(ctx, existing, in.HTML); err != nil {
			return SaveResult{}, err
		}
		return SaveResult{ReportID: existing.ID, Title: existing.Title, Created: false, HTMLBytes: size}, nil
	}
	id, err := ids.New()
	if err != nil {
		return SaveResult{}, fmt.Errorf("create report id: %w", err)
	}
	item := Metadata{ID: id, ProjectID: in.ProjectID, Title: in.Title, Summary: in.Summary, Skill: in.Skill, TemplateID: in.TemplateID, CreatedBy: in.CreatedBy, CreatedByUserID: in.CreatedByUserID, SizeBytes: size, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.Insert(ctx, item, in.HTML); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{ReportID: id, Title: in.Title, Created: true, HTMLBytes: size}, nil
}

func (s *Service) List(ctx context.Context, projectID string, limit, offset int) (ListResult, error) {
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit || offset < 0 {
		return ListResult{}, fail("VALIDATION_ERROR", "limit must be 1 to 50 and offset must be non-negative.")
	}
	rows, err := s.Store.List(ctx, projectID, limit, offset)
	if err != nil {
		return ListResult{}, err
	}
	total, err := s.Store.Count(ctx, projectID)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Reports: rows, TotalCount: total, RowCount: len(rows), Remaining: max(0, MaxReportsPerProject-total)}, nil
}
func (s *Service) Get(ctx context.Context, projectID, id string) (Metadata, error) {
	item, err := s.Store.Get(ctx, projectID, id)
	if errors.Is(err, ErrNotFound) {
		return Metadata{}, fail("NOT_FOUND", "Report does not exist in this project.")
	}
	if err != nil {
		return Metadata{}, err
	}
	return item, nil
}
func (s *Service) GetWithHTML(ctx context.Context, projectID, id string) (Metadata, string, error) {
	item, html, err := s.Store.GetHTML(ctx, projectID, id)
	if errors.Is(err, ErrNotFound) {
		return Metadata{}, "", fail("NOT_FOUND", "Report does not exist in this project.")
	}
	if err != nil {
		return Metadata{}, "", err
	}
	return item, html, nil
}
func (s *Service) Delete(ctx context.Context, projectID, id string) error {
	ok, err := s.Store.Delete(ctx, projectID, id)
	if err != nil {
		return err
	}
	if !ok {
		return fail("NOT_FOUND", "Report does not exist in this project.")
	}
	return nil
}

func (s *Service) Share(ctx context.Context, projectID, id string) (Metadata, error) {
	if !s.Hosted {
		return Metadata{}, fail("VALIDATION_ERROR", "Public report sharing is only available on hosted Seomarine.")
	}
	item, err := s.Get(ctx, projectID, id)
	if err != nil {
		return Metadata{}, err
	}
	if item.ShareToken != nil && *item.ShareToken != "" {
		return item, nil
	}
	token, err := s.shareToken()
	if err != nil {
		return Metadata{}, fmt.Errorf("mint report share token: %w", err)
	}
	stamp := utcStamp(s.now())
	created, err := s.Store.SetShare(ctx, projectID, id, &token, &stamp)
	if err != nil {
		return Metadata{}, err
	}
	if !created {
		return s.Get(ctx, projectID, id)
	}
	item.ShareToken = &token
	item.SharedAt = &stamp
	return item, nil
}
func (s *Service) Unshare(ctx context.Context, projectID, id string) (Metadata, error) {
	item, err := s.Get(ctx, projectID, id)
	if err != nil {
		return Metadata{}, err
	}
	if item.ShareToken == nil {
		return item, nil
	}
	if _, err := s.Store.SetShare(ctx, projectID, id, nil, nil); err != nil {
		return Metadata{}, err
	}
	item.ShareToken = nil
	item.SharedAt = nil
	return item, nil
}
func (s *Service) GetShared(ctx context.Context, token string) (SharedReport, error) {
	if !validShareToken(token) {
		return SharedReport{}, fail("NOT_FOUND", "Shared report not found.")
	}
	item, err := s.Store.GetShared(ctx, token)
	if errors.Is(err, ErrNotFound) {
		return SharedReport{}, fail("NOT_FOUND", "Shared report not found.")
	}
	return item, err
}

func (s *Service) ListTemplates(ctx context.Context, projectID string) (ListTemplateResult, error) {
	rows, err := s.Store.ListTemplates(ctx, projectID)
	if err != nil {
		return ListTemplateResult{}, err
	}
	return ListTemplateResult{Templates: rows, Remaining: max(0, MaxTemplatesPerProject-len(rows))}, nil
}
func (s *Service) GetTemplate(ctx context.Context, projectID, id string) (Template, error) {
	item, err := s.Store.GetTemplate(ctx, projectID, id)
	if errors.Is(err, ErrNotFound) {
		return Template{}, fail("NOT_FOUND", "Report template does not exist in this project.")
	}
	return item, err
}

type SaveTemplateInput struct{ ProjectID, TemplateID, Name, Description, Instructions, CreatedBy, CreatedByUserID string }
type SaveTemplateResult struct {
	TemplateID string `json:"templateId"`
	Name       string `json:"name"`
	Created    bool   `json:"created"`
}

func (s *Service) SaveTemplate(ctx context.Context, in SaveTemplateInput) (SaveTemplateResult, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.Instructions = strings.TrimSpace(in.Instructions)
	if in.Name == "" || in.Description == "" || in.Instructions == "" {
		return SaveTemplateResult{}, fail("VALIDATION_ERROR", "Template name, description, and instructions are required.")
	}
	if chars(in.Name) > MaxTemplateNameChars || chars(in.Description) > MaxTemplateDescChars || chars(in.Instructions) > MaxTemplateBriefChars {
		return SaveTemplateResult{}, fail("VALIDATION_ERROR", "Template name, description, or instructions exceed their character limit.")
	}
	rows, err := s.Store.ListTemplates(ctx, in.ProjectID)
	if err != nil {
		return SaveTemplateResult{}, err
	}
	var current *Template
	for i := range rows {
		if in.TemplateID != "" && rows[i].ID == in.TemplateID {
			current = &rows[i]
		}
		if strings.EqualFold(rows[i].Name, in.Name) && (in.TemplateID == "" || rows[i].ID != in.TemplateID) {
			return SaveTemplateResult{}, fail("VALIDATION_ERROR", "A template with this name already exists in the project.")
		}
	}
	if in.TemplateID != "" && current == nil {
		return SaveTemplateResult{}, fail("NOT_FOUND", "Report template does not exist in this project.")
	}
	created := current == nil
	var item Template
	if current != nil {
		item = *current
		item.Name = in.Name
		item.Description = in.Description
		item.Instructions = in.Instructions
		item.UpdatedAt = utcStamp(s.now())
		ok, err := s.Store.UpdateTemplate(ctx, item)
		if err != nil {
			return SaveTemplateResult{}, err
		}
		if !ok {
			return SaveTemplateResult{}, fail("NOT_FOUND", "Report template does not exist in this project.")
		}
	} else {
		if len(rows) >= MaxTemplatesPerProject {
			return SaveTemplateResult{}, fail("VALIDATION_ERROR", "This project has reached the report template limit.")
		}
		id, err := ids.New()
		if err != nil {
			return SaveTemplateResult{}, fmt.Errorf("create report template id: %w", err)
		}
		stamp := utcStamp(s.now())
		item = Template{ID: id, ProjectID: in.ProjectID, Name: in.Name, Description: in.Description, Instructions: in.Instructions, CreatedBy: in.CreatedBy, CreatedByUserID: in.CreatedByUserID, CreatedAt: stamp, UpdatedAt: stamp}
		if err := s.Store.InsertTemplate(ctx, item); err != nil {
			return SaveTemplateResult{}, err
		}
	}
	return SaveTemplateResult{TemplateID: item.ID, Name: item.Name, Created: created}, nil
}
func (s *Service) DeleteTemplate(ctx context.Context, projectID, id string) error {
	ok, err := s.Store.DeleteTemplate(ctx, projectID, id)
	if err != nil {
		return err
	}
	if !ok {
		return fail("NOT_FOUND", "Report template does not exist in this project.")
	}
	return nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s *Service) shareToken() (string, error) {
	if s.NewShareToken != nil {
		return s.NewShareToken()
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func validShareToken(token string) bool {
	if len(token) != 32 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == 24
}
func chars(value string) int  { return len(utf16.Encode([]rune(value))) }
func kbUp(bytes int) string   { return fmt.Sprintf("%d KB", (bytes+999)/1000) }
func kbDown(bytes int) string { return fmt.Sprintf("%d KB", bytes/1000) }
