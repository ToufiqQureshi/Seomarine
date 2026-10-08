package auth

import "context"

type (
	userContextKey                struct{}
	projectOrganizationContextKey struct{}
)

// WithUser adds the authenticated session user to a request context.
func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

// UserFromContext returns the authenticated session user, when present.
func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}

// WithProjectOrganization adds the organization that owns the request's
// project, as found by AuthorizeProject.
func WithProjectOrganization(ctx context.Context, organizationID string) context.Context {
	return context.WithValue(ctx, projectOrganizationContextKey{}, organizationID)
}

// ProjectOrganizationFromContext returns the organization that owns the
// request's project, when the request went through project authorization.
func ProjectOrganizationFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(projectOrganizationContextKey{}).(string)
	return id, ok
}
