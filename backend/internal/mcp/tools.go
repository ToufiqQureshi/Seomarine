package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/billing"
	"sync"

	"github.com/toufiqqureshi/seomarine/backend/internal/projectcontext"
)

// emptyObjectSchema is the JSON Schema for a tool with no inputs (z.object({})).
var emptyObjectSchema = json.RawMessage(`{"type":"object","properties":{}}`)

func readOnlyAnnotations() map[string]any {
	return map[string]any{"readOnlyHint": true, "openWorldHint": false, "destructiveHint": false}
}

func mutatingAnnotations() map[string]any {
	return map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": false}
}

func whoamiTool() *tool {
	return &tool{
		Name:        "whoami",
		Title:       "Who am I",
		Description: "Confirms the connected Seomarine account, server mode, token scopes, and current credit balance when the user asks to check their account or connection. Uses no credits — does not call DataForSEO.",
		InputSchema: emptyObjectSchema,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{
			"userEmail":{"type":"string"},
			"scopes":{"type":"array","items":{"type":"string"}},
			"mode":{"type":"string","enum":["hosted","self-hosted"]},
			"creditsRemaining":{"type":["number","null"]},
			"meta":{"type":"object","additionalProperties":true}
		},"additionalProperties":true}`),
		Annotations: readOnlyAnnotations(),
		Handler:     handleWhoami,
	}
}

func handleWhoami(ctx context.Context, _ json.RawMessage, env *callEnv) (*callResult, error) {
	auth := env.auth
	hosted := env.deps.Hosted
	mode := "self-hosted"
	if hosted {
		mode = "hosted"
	}
	scopes := auth.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	scopeText := "none"
	if len(scopes) > 0 {
		scopeText = strings.Join(scopes, ", ")
	}

	var creditsRemaining *int
	if hosted && env.deps.AutumnCredits != nil && auth.OrganizationID != "" {
		var base, topup *float64
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			base, _ = env.deps.AutumnCredits.Balance(ctx, auth.OrganizationID, billing.AutumnSEODataBalanceFeatureID)
		}()
		go func() {
			defer wg.Done()
			topup, _ = env.deps.AutumnCredits.Balance(ctx, auth.OrganizationID, billing.AutumnSEOTopupBalanceFeatureID)
		}()
		wg.Wait()
		if base != nil || topup != nil {
			total := 0.0
			if base != nil {
				total += *base
			}
			if topup != nil {
				total += *topup
			}
			credits := int(total)
			creditsRemaining = &credits
		}
	}
	lines := []string{
		"Account: " + auth.UserEmail,
		"Mode: " + mode,
		"Scopes: " + scopeText,
	}
	if hosted {
		creditText := "unknown"
		if creditsRemaining != nil {
			creditText = fmt.Sprintf("%d", *creditsRemaining)
		}
		lines = append(lines, "Credits remaining: "+creditText)
	}
	return mcpResponse(strings.Join(lines, "\n"), map[string]any{
		"userEmail":        auth.UserEmail,
		"scopes":           scopes,
		"mode":             mode,
		"creditsRemaining": creditsRemaining,
	}, metaFields{CreditsRemaining: creditsRemaining}), nil
}

func listProjectsTool() *tool {
	return &tool{
		Name:        "list_projects",
		Title:       "List projects",
		Description: "Lists the user's projects. Uses no credits — does not call DataForSEO. Use this whenever you need a `projectId` for another Seomarine tool. Returns an array of {id, name, domain, locationCode, languageCode}; pass the `id` value as `projectId`. locationCode/languageCode are the project's default market — tools fall back to them when a call omits location/language args. When the user belongs to several organizations, each project is labeled with its organization and organizationId (pass that to create_project).",
		InputSchema: emptyObjectSchema,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{
			"projects":{"type":"array","items":{"type":"object","properties":{
				"id":{"type":"string"},"name":{"type":"string"},
				"domain":{"type":["string","null"]},
				"locationCode":{"type":"number"},"languageCode":{"type":"string"},
				"url":{"type":"string"},
				"organization":{"type":"string"},"organizationId":{"type":"string"}
			},"additionalProperties":true}},
			"meta":{"type":"object","additionalProperties":true}
		},"additionalProperties":true}`),
		Annotations: readOnlyAnnotations(),
		Handler:     handleListProjects,
	}
}

type listedProject struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Domain         *string `json:"domain"`
	LocationCode   int     `json:"locationCode"`
	LanguageCode   string  `json:"languageCode"`
	URL            string  `json:"url"`
	Organization   *string `json:"organization,omitempty"`
	OrganizationID *string `json:"organizationId,omitempty"`
}

func handleListProjects(ctx context.Context, _ json.RawMessage, env *callEnv) (*callResult, error) {
	auth := env.auth
	baseURL := auth.BaseURL

	listed := []listedProject{}
	if auth.OrgScope != "user" {
		projects, err := listProjects(ctx, env.deps.DB, auth.OrganizationID)
		if err != nil {
			return nil, err
		}
		for _, p := range projects {
			listed = append(listed, toListedProject(p, baseURL, nil, nil))
		}
	} else {
		memberships, err := listMembershipsForUser(ctx, env.deps.DB, auth.UserID)
		if err != nil {
			return nil, err
		}
		for _, m := range memberships {
			projects, err := listProjects(ctx, env.deps.DB, m.organizationID)
			if err != nil {
				return nil, err
			}
			orgName, orgID := m.organizationName, m.organizationID
			for _, p := range projects {
				listed = append(listed, toListedProject(p, baseURL, &orgName, &orgID))
			}
		}
	}

	lines := make([]string, 0, len(listed)+1)
	if len(listed) == 0 {
		lines = append(lines, "No projects yet. Create one in the dashboard.")
	} else {
		for _, p := range listed {
			line := "- " + p.ID + "  " + p.Name
			if p.Domain != nil && *p.Domain != "" {
				line += " (" + *p.Domain + ")"
			}
			if p.Organization != nil {
				line += "  organization:" + *p.Organization + " [" + *p.OrganizationID + "]"
			}
			line += fmt.Sprintf("  market:%d/%s", p.LocationCode, p.LanguageCode)
			lines = append(lines, line)
		}
	}

	return mcpResponse(fmt.Sprintf("Projects (%d):\n%s", len(listed), strings.Join(lines, "\n")),
		map[string]any{"projects": listed},
		metaFields{URL: buildDashboardURL(baseURL, "/", nil)}), nil
}

func toListedProject(p project, baseURL string, organization, organizationID *string) listedProject {
	return listedProject{
		ID:             p.ID,
		Name:           p.Name,
		Domain:         p.Domain,
		LocationCode:   p.LocationCode,
		LanguageCode:   p.LanguageCode,
		URL:            buildDashboardURL(baseURL, "/p/"+p.ID, nil),
		Organization:   organization,
		OrganizationID: organizationID,
	}
}

var createProjectInputSchema = json.RawMessage(`{"type":"object","properties":{
	"name":{"type":"string","minLength":1,"maxLength":120,"description":"Project name (1-120 characters)."},
	"domain":{"type":"string","maxLength":255,"description":"Optional root domain for the project, e.g. \"example.com\" (host only, no scheme or path). Sets the default target for domain, backlink, and rank tools."},
	"locationCode":{"type":"number","description":"Optional DataForSEO location code for the project's default market (e.g. 2840 = United States, 2504 = Morocco). Falls back to the organization default when omitted."},
	"languageCode":{"type":"string","description":"Optional language code (e.g. \"en\", \"fr\"). Requires locationCode; derived from the location when omitted."},
	"organizationId":{"type":"string","minLength":1,"description":"Organization id to create the project in. Required when the user belongs to more than one organization — omitting it returns the list; confirm the choice with the user before retrying."}
},"required":["name"]}`)

func createProjectTool() *tool {
	return &tool{
		Name:        "create_project",
		Title:       "Create project",
		Description: "Create a new project in the user's organization. Uses no credits — does not call DataForSEO. Provide a name, and optionally a domain and default market (locationCode/languageCode; a languageCode requires a locationCode). Returns the created {id, name, domain, locationCode, languageCode, url}; pass the returned `id` as `projectId` to other Seomarine tools. Call list_projects first to avoid creating a duplicate.",
		InputSchema: createProjectInputSchema,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{
			"project":{"type":"object","properties":{
				"id":{"type":"string"},"name":{"type":"string"},
				"domain":{"type":["string","null"]},
				"locationCode":{"type":"number"},"languageCode":{"type":"string"},
				"url":{"type":"string"}
			},"additionalProperties":true},
			"meta":{"type":"object","additionalProperties":true}
		},"additionalProperties":true}`),
		Annotations: mutatingAnnotations(),
		Handler:     handleCreateProject,
	}
}

type createProjectArgs struct {
	Name           string  `json:"name"`
	Domain         *string `json:"domain"`
	LocationCode   *int    `json:"locationCode"`
	LanguageCode   *string `json:"languageCode"`
	OrganizationID *string `json:"organizationId"`
}

func handleCreateProject(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args createProjectArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, newAppErrorf("VALIDATION_ERROR", "Invalid input: "+err.Error())
	}
	if strings.TrimSpace(args.Name) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "Project name is required")
	}
	if len([]rune(strings.TrimSpace(args.Name))) > 120 {
		return nil, newAppErrorf("VALIDATION_ERROR", "Project name is required (1-120 characters)")
	}
	name := strings.TrimSpace(args.Name)

	// A language requires a location (market pair rule).
	if args.LanguageCode != nil && *args.LanguageCode != "" && args.LocationCode == nil {
		return nil, newAppErrorf("VALIDATION_ERROR", "A language requires a location.")
	}

	targetOrg, role, err := resolveTargetOrganization(ctx, env, args.OrganizationID)
	if err != nil {
		return nil, err
	}
	if !hasProjectCreatePermission(role) {
		return nil, newAppErrorf("FORBIDDEN", "Your organization role does not allow this action.")
	}

	var domain *string
	if args.Domain != nil && strings.TrimSpace(*args.Domain) != "" {
		normalized, err := normalizeProjectDomain(*args.Domain)
		if err != nil {
			return nil, newAppErrorf("VALIDATION_ERROR", "Enter a valid domain, like acme.com.")
		}
		domain = normalized
	}
	market, err := resolveMarketInput(args.LocationCode, args.LanguageCode)
	if err != nil {
		return nil, newAppErrorf("VALIDATION_ERROR", err.Error())
	}

	p, err := createProject(ctx, env.deps.DB, targetOrg, name, domain, market)
	if err != nil {
		return nil, err
	}

	domainText := ""
	if p.Domain != nil {
		domainText = " (" + *p.Domain + ")"
	}
	return mcpResponse(
		fmt.Sprintf("Created project %s  %s%s  market:%d/%s", p.ID, p.Name, domainText, p.LocationCode, p.LanguageCode),
		map[string]any{"project": map[string]any{
			"id": p.ID, "name": p.Name, "domain": p.Domain,
			"locationCode": p.LocationCode, "languageCode": p.LanguageCode,
			"url": buildDashboardURL(env.auth.BaseURL, "/p/"+p.ID, nil),
		}},
		metaFields{URL: buildDashboardURL(env.auth.BaseURL, "/p/"+p.ID, nil)}), nil
}

// resolveTargetOrganization picks the org a new project is created in. Pinned
// credentials stay in their org; user-scoped ones with one membership take it,
// with none fail, and with several return the member how to choose.
func resolveTargetOrganization(ctx context.Context, env *callEnv, organizationID *string) (string, string, error) {
	auth := env.auth
	if auth.OrgScope != "user" {
		if organizationID != nil && *organizationID != "" && *organizationID != auth.OrganizationID {
			return "", "", newAppErrorf("FORBIDDEN", "This connection is bound to a single organization — omit organizationId.")
		}
		return auth.OrganizationID, auth.Role, nil
	}
	memberships, err := listMembershipsForUser(ctx, env.deps.DB, auth.UserID)
	if err != nil {
		return "", "", err
	}
	if organizationID != nil && *organizationID != "" {
		for _, m := range memberships {
			if m.organizationID == *organizationID {
				return m.organizationID, m.role, nil
			}
		}
		return "", "", newAppErrorf("FORBIDDEN", "The user is not a member of that organization.")
	}
	if len(memberships) == 1 {
		return memberships[0].organizationID, memberships[0].role, nil
	}
	if len(memberships) == 0 {
		return "", "", newAppError("FORBIDDEN")
	}
	lines := make([]string, 0, len(memberships))
	for _, m := range memberships {
		lines = append(lines, "- "+m.organizationID+"  "+m.organizationName)
	}
	return "", "", newAppErrorf("VALIDATION_ERROR",
		fmt.Sprintf("The user belongs to %d organizations. Ask the user which organization this project should be created in, then retry with organizationId set:\n%s", len(memberships), strings.Join(lines, "\n")))
}

// hasProjectCreatePermission mirrors better-auth's owner/admin project:create
// gate. Roles are comma-joined when multiple.
func hasProjectCreatePermission(role string) bool {
	for _, part := range strings.Split(role, ",") {
		if name := strings.TrimSpace(part); name == "owner" || name == "admin" {
			return true
		}
	}
	return false
}

var getProjectContextInputSchema = json.RawMessage(`{"type":"object","properties":{
	"projectId":{"type":"string","minLength":1,"description":"Required. The Seomarine project ID to scope this call to. Get one from list_projects."}
},"required":["projectId"]}`)

func getProjectContextTool() *tool {
	return &tool{
		Name:        "get_project_context",
		Title:       "Get project context",
		Description: "Reads a project's shared memory: business overview, current goal, positioning, writing preferences, custom sections, competitors, key pages, and the recent research log. Uses no credits. Call this before SEO work to ground it in what the user already told Seomarine, and check the research log before re-buying research. Sections listed as missing are the ones worth filling with update_project_context.",
		InputSchema: getProjectContextInputSchema,
		OutputSchema: json.RawMessage(`{"type":"object","properties":{
			"sections":{"type":"array","items":{"type":"object","additionalProperties":true}},
			"missingSections":{"type":"array","items":{"type":"string"}},
			"customSections":{"type":"array","items":{"type":"object","additionalProperties":true}},
			"competitors":{"type":"array","items":{"type":"object","additionalProperties":true}},
			"keyPages":{"type":"array","items":{"type":"object","additionalProperties":true}},
			"researchLog":{"type":"array","items":{"type":"object","additionalProperties":true}},
			"reportTemplates":{"type":"array","items":{"type":"object","additionalProperties":true}},
			"meta":{"type":"object","additionalProperties":true}
		},"additionalProperties":true}`),
		Annotations: readOnlyAnnotations(),
		Handler:     handleGetProjectContext,
	}
}

type projectContextArgs struct {
	ProjectID string `json:"projectId"`
}

func handleGetProjectContext(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args projectContextArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, newAppErrorf("VALIDATION_ERROR", "Invalid input: "+err.Error())
	}
	if strings.TrimSpace(args.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.ProjectContext == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Project context is not available.")
	}
	loaded, err := env.deps.ProjectContext.Get(ctx, access.Project.ID)
	if err != nil {
		return nil, err
	}
	return mcpResponse(projectcontext.RenderMarkdown(loaded), loaded, metaFields{
		ProjectID: access.Project.ID,
		URL:       buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/context", nil),
	}), nil
}
