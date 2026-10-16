# Projects and project context: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Creates, switches, updates, archives/restores projects and manages shared project memory.

## Decisions to preserve

- Project ID from the browser is untrusted; each handler must re-check membership and organization scope. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Create/archive/restore must preserve plan limits and data ownership. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.
- Project context is shared with SAM/MCP; retain size and count caps to limit prompt growth and storage abuse. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.

## Verification pointers

- `projectApi.test.ts`
- `project-context/EditableListSection.test.ts`
