# Projects and project context: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. Project switcher loads the caller’s available projects; create/edit/archive actions use `projectApi.ts`.
2. Selected project ID scopes feature queries through the app route/context.
3. Project Context reads typed/custom sections; updates are applied as one bounded batch through `projectContextApi.ts`.
4. Go enforces organization membership, plan limits, caps and canonical URLs.

## Implementation map

- `ProjectSwitcher.tsx`
- `projectApi.ts`
- `project-context/ProjectContextPage.tsx`
- `projectContextApi.ts`
- `backend/internal/projects`
- `backend/internal/projectcontext`

## API and service dependencies

Go packages: `projects` and `projectcontext`; project context is also consumed by SAM and MCP.

## Cost, security and failure boundaries

- Project ID from the browser is untrusted; each handler must re-check membership and organization scope.
- Create/archive/restore must preserve plan limits and data ownership.
- Project context is shared with SAM/MCP; retain size and count caps to limit prompt growth and storage abuse.
