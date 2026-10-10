package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/projectcontext"
)

var updateProjectContextSchema = json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"updates":{"type":"array","minItems":1,"maxItems":50,"items":{"oneOf":[{"type":"object","properties":{"section":{"type":"string","enum":["business_overview","current_goal","positioning","writing_preferences"]},"content":{"type":"string"}},"required":["section","content"],"additionalProperties":false},{"type":"object","properties":{"customSection":{"type":"string","pattern":"^[a-z0-9]+(?:-[a-z0-9]+)*$"},"title":{"type":"string","minLength":1,"maxLength":120},"content":{"type":"string"}},"required":["customSection","content"],"additionalProperties":false},{"type":"object","properties":{"deleteCustomSection":{"type":"string","pattern":"^[a-z0-9]+(?:-[a-z0-9]+)*$"}},"required":["deleteCustomSection"],"additionalProperties":false},{"type":"object","properties":{"addCompetitors":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","properties":{"domain":{"type":"string","minLength":1,"maxLength":255},"name":{"type":"string","maxLength":120},"notes":{"type":"string","maxLength":500}},"required":["domain"],"additionalProperties":false}}},"required":["addCompetitors"],"additionalProperties":false},{"type":"object","properties":{"removeCompetitors":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string","minLength":1}}},"required":["removeCompetitors"],"additionalProperties":false},{"type":"object","properties":{"addKeyPages":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","properties":{"url":{"type":"string","minLength":1,"maxLength":2048},"role":{"type":"string","enum":["hub","spoke","money","other"]},"topic":{"type":"string","maxLength":200},"notes":{"type":"string","maxLength":500}},"required":["url"],"additionalProperties":false}}},"required":["addKeyPages"],"additionalProperties":false},{"type":"object","properties":{"removeKeyPages":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string","minLength":1}}},"required":["removeKeyPages"],"additionalProperties":false},{"type":"object","properties":{"removeResearchLog":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string","minLength":1}}},"required":["removeResearchLog"],"additionalProperties":false},{"type":"object","properties":{"appendResearchLog":{"type":"object","properties":{"summary":{"type":"string","minLength":1,"maxLength":1000}},"required":["summary"],"additionalProperties":false}},"required":["appendResearchLog"],"additionalProperties":false}]}}},"required":["projectId","updates"],"additionalProperties":false}`)

func updateProjectContextTool() *tool {
	return &tool{Name: "update_project_context", Title: "Update project context", Description: "Updates the shared memory for one project: typed or custom sections, competitors, key pages, and the 90-day research log. Uses no credits. Only save facts or edits the user asked SAM to remember or change; do not infer permanent preferences from a one-off request. The entire batch is validated and committed atomically.", InputSchema: updateProjectContextSchema, OutputSchema: json.RawMessage(`{"type":"object","properties":{"sections":{"type":"array"},"missingSections":{"type":"array"},"customSections":{"type":"array"},"competitors":{"type":"array"},"keyPages":{"type":"array"},"researchLog":{"type":"array"},"reportTemplates":{"type":"array"}},"additionalProperties":true}`), Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": false}, Handler: handleUpdateProjectContext}
}

type updateProjectContextArgs struct {
	ProjectID string            `json:"projectId"`
	Updates   []json.RawMessage `json:"updates"`
}

func handleUpdateProjectContext(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var in updateProjectContextArgs
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.ProjectID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and updates are required.")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and updates are required.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.ProjectContext == nil {
		return nil, newAppErrorf("INTERNAL_ERROR", "Project context is not available.")
	}
	result, err := env.deps.ProjectContext.Apply(ctx, access.Project.ID, in.Updates, "mcp")
	if err != nil {
		var typed *projectcontext.Error
		if errors.As(err, &typed) {
			return nil, newAppErrorf(typed.Code, typed.Message)
		}
		return nil, fmt.Errorf("update project context: %w", err)
	}
	return mcpResponse("Updated project context for "+access.Project.Name+".", result, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/context", nil)}), nil
}
