package mcp

import "context"

// appError is a tool error with a stable code. The code doubles as the message,
// matching the legacy AppError where a bare code is both.
type appError struct {
	code    string
	message string
}

func (e *appError) Error() string {
	if e.message != "" {
		return e.message
	}
	return e.code
}

func newAppError(code string) *appError { return &appError{code: code} }

func newAppErrorf(code, message string) *appError {
	return &appError{code: code, message: message}
}

// projectAccess is the authorized project plus the credential rebound to the
// project's organization.
type projectAccess struct {
	Project project
	Auth    Auth
}

// authorizeProject applies the legacy withMcpProjectAuth rules. For user-scoped
// credentials it derives the org from the project row and checks membership
// (one FORBIDDEN for both unknown-project and no-membership, so probing cannot
// tell them apart); for pinned credentials it checks the project against the
// token's org.
func (h *handler) authorizeProject(ctx context.Context, auth Auth, projectID string) (projectAccess, error) {
	if auth.OrgScope == "user" {
		p, ok, err := getProjectWithOrganization(ctx, h.deps.DB, projectID)
		if err != nil {
			return projectAccess{}, err
		}
		if !ok {
			return projectAccess{}, newAppError("FORBIDDEN")
		}
		role, member, err := getMembership(ctx, h.deps.DB, auth.UserID, p.OrganizationID)
		if err != nil {
			return projectAccess{}, err
		}
		if !member {
			return projectAccess{}, newAppError("FORBIDDEN")
		}
		rebound := auth
		rebound.OrganizationID = p.OrganizationID
		rebound.Role = role
		return projectAccess{Project: *p, Auth: rebound}, nil
	}

	p, ok, err := getProjectForOrganization(ctx, h.deps.DB, auth.OrganizationID, projectID)
	if err != nil {
		return projectAccess{}, err
	}
	if !ok {
		return projectAccess{}, newAppError("FORBIDDEN")
	}
	return projectAccess{Project: *p, Auth: auth}, nil
}
