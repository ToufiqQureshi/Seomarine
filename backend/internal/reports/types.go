package reports

import (
	"errors"
	"time"
)

const (
	// MaxHTMLBytes is the largest HTML report document accepted.
	MaxHTMLBytes = 500_000
	// MaxReportsPerProject is the report storage limit for a project.
	MaxReportsPerProject = 10_000
	// MaxReportBytesPerOrg is the report HTML storage limit for an organization.
	MaxReportBytesPerOrg = 5_000_000_000
	// DefaultListLimit is the default report page size.
	DefaultListLimit = 20
	// MaxListLimit is the largest report page size accepted.
	MaxListLimit = 50
	// MaxTitleCharacters is the largest allowed report title length.
	MaxTitleCharacters = 120
	// MaxSkillCharacters is the largest allowed report skill label length.
	MaxSkillCharacters = 60
	// MaxSummaryCharacters is the largest allowed report summary length.
	MaxSummaryCharacters = 2_500
	// MaxTemplatesPerProject is the maximum number of report templates per project.
	MaxTemplatesPerProject = 10
	// MaxTemplateNameChars is the maximum report template name length.
	MaxTemplateNameChars = 80
	// MaxTemplateDescChars is the maximum report template description length.
	MaxTemplateDescChars = 200
	// MaxTemplateBriefChars is the maximum report template instruction length.
	MaxTemplateBriefChars = 3_000
)

// ErrNotFound indicates that a report or template does not exist.
var ErrNotFound = errors.New("report or template not found")

// Error is a stable error that can be returned to HTTP and MCP callers.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Message }

func fail(code, message string) error { return &Error{Code: code, Message: message} }

// Metadata describes a saved report without its HTML document.
type Metadata struct {
	ID              string  `json:"id"`
	ProjectID       string  `json:"projectId"`
	Title           string  `json:"title"`
	Summary         string  `json:"summary"`
	Skill           *string `json:"skill"`
	TemplateID      *string `json:"templateId"`
	CreatedBy       string  `json:"createdBy"`
	CreatedByUserID string  `json:"createdByUserId"`
	SizeBytes       int     `json:"sizeBytes"`
	ShareToken      *string `json:"shareToken"`
	SharedAt        *string `json:"sharedAt"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

// Template is a reusable project report template.
type Template struct {
	ID              string `json:"id"`
	ProjectID       string `json:"projectId"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Instructions    string `json:"instructions"`
	CreatedBy       string `json:"createdBy"`
	CreatedByUserID string `json:"createdByUserId"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

// SharedReport contains a report and its public sharing context.
type SharedReport struct {
	Metadata
	HTML           string  `json:"html"`
	OrganizationID string  `json:"organizationId"`
	ProjectDomain  *string `json:"projectDomain"`
	Archived       bool    `json:"archived"`
}

func utcStamp(now time.Time) string { return now.UTC().Format("2006-01-02T15:04:05.000Z") }
