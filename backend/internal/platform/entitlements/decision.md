# Plan entitlement interface: decisions

> Internal guide for developers and coding agents. Update with code changes.

## Responsibility

Defines the check contract for organization plan entitlements; `AllowAll` is a migration stub.

## Decisions to preserve

- Do not mistake `AllowAll` for production entitlement enforcement; `rg AllowAll backend` currently finds no production wiring. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Pass the organization responsible for usage, not caller-supplied IDs. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Honor context cancellation and keep the contract stable for callers. _Why:_ UI and external clients depend on this contract; changing it without coordinated callers creates avoidable breakage.

## Cost and risk boundary

Feature services and plan/billing integration.
