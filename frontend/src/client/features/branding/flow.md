# Report branding: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. Settings or a report template loads the saved organization branding.
2. An owner/admin changes fields or resets to defaults; `brandingApi.ts` posts the mutation.
3. Go `branding` checks role and plan entitlement, validates fields and sanitizes SVG data URLs before save.
4. Reports read the organization-scoped branding record when rendering.

## Implementation map

- `brandingApi.ts`
- `../settings/BrandingSettings.tsx`
- `backend/internal/branding/svg.go`
- `backend/internal/reports`

## API and service dependencies

Go package: `backend/internal/branding`; report rendering consumes its persisted configuration.

## Cost, security and failure boundaries

- Branding is organization-scoped; never read or write another organization’s configuration.
- SVG input is active content: preserve sanitization and reject external references/scripts.
- A displayed white-label entitlement must match the server plan gate.
