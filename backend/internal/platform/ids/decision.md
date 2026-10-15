# Shared random IDs: decisions

> Internal guide for developers and coding agents. Update with code changes.

## Responsibility

Generates UUID v4 identifiers for Go-owned records.

## Decisions to preserve

- Use cryptographic randomness and return entropy errors; never silently fall back to predictable IDs. _Why:_ Explicit failure behavior helps callers recover safely instead of treating an error as a valid result.
- Keep UUID format compatible with existing IDs and database constraints. _Why:_ UI and external clients depend on this contract; changing it without coordinated callers creates avoidable breakage.

## Cost and risk boundary

Go `crypto/rand`; no external provider or stored state.
