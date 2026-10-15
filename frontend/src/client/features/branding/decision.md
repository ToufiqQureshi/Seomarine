# Report branding: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Lets an eligible workspace configure brand elements used in white-label report experiences.

## Decisions to preserve

- Branding is organization-scoped; never read or write another organization’s configuration. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- SVG input is active content: preserve sanitization and reject external references/scripts. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.
- A displayed white-label entitlement must match the server plan gate. _Why:_ A single server-owned source keeps UI access, checkout and limits aligned even if client state is stale or modified.

## Verification pointers

- `brandingApi.test.ts`
