package reports

import (
	"errors"
	"time"
)

const (
	MaxHTMLBytes           = 500_000
	MaxReportsPerProject   = 10_000
	MaxReportBytesPerOrg   = 5_000_000_000
	DefaultListLimit       = 20
	MaxListLimit           = 50
	MaxTitleCharacters     = 120
	MaxSkillCharacters     = 60
	MaxSummaryCharacters   = 2_500
	MaxTemplatesPerProject = 10
	MaxTemplateNameChars   = 80
	MaxTemplateDescChars   = 200
	MaxTemplateBriefChars  = 3_000
)

var ErrNotFound = errors.New("report or template not found")

// Error is a stable error that can be returned to HTTP and MCP callers.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Message }

func fail(code, message string) error { return &Error{Code: code, Message: message} }

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

type SharedReport struct {
	Metadata
	HTML           string  `json:"html"`
	OrganizationID string  `json:"organizationId"`
	ProjectDomain  *string `json:"projectDomain"`
	Archived       bool    `json:"archived"`
}

func utcStamp(now time.Time) string { return now.UTC().Format("2006-01-02T15:04:05.000Z") }
