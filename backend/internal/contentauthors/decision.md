# Project content authors: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Stores editorial author profiles used for draft bylines and structured article author data.

## Decisions to preserve

- Author profiles are editorial identities, not login accounts. _Why:_ Keeping product meaning aligned with supported behavior avoids misleading customers and incorrect agent responses.
- Every read/write is project-scoped; validate lengths and fields before persistence. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Only include author schema fields that the user supplied and the product supports. _Why:_ UI and external clients depend on this contract; changing it without coordinated callers creates avoidable breakage.

## Dependencies

Postgres `go_content_authors`; content editor and site publisher.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
