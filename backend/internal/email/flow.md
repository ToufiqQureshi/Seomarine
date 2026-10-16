# Transactional email transport: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** No public feature route; called by auth and team services.
- **Source:** `loops.go`

## Request or job flow

Feature service constructs a bounded transactional payload; email package adds auth/config and deadline, calls Loops and returns a safe unavailable/error result.

## Dependencies and cost boundary

Loops transactional API and secret environment configuration.

## Failure, security and data invariants

- No marketing enrollment or contact-list side effects from transactional sends.
- Use request deadlines and context cancellation; never log recipient secrets or provider auth.
- Provider failure must be handled by the calling flow without claiming that mail was delivered.
