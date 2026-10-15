# Report branding: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Stores organization branding used for white-label reports and validates active SVG content.

## Decisions to preserve

- Keep organization scoping and role/plan checks server-side. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Reject executable or externally referenced SVG content; do not relax the sanitizer to accept a logo. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.
- Public report output must escape text and only render validated branding. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.

## Dependencies

Postgres organization branding; reports renderer; plan catalog.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
