// Package entitlements defines the plan-limit check used before billable work.
package entitlements

import "context"

// Checker reports whether an organization may create or consume a resource.
// Implementations should return a stable error that callers can map to HTTP 402.
type Checker interface {
	Check(ctx context.Context, organizationID, resource string) error
}

// AllowAll is the migration stub. Billing can replace it with a database-backed
// checker without changing feature service contracts.
type AllowAll struct{}

// Check always allows the action.
func (AllowAll) Check(ctx context.Context, _, _ string) error {
	return ctx.Err()
}
