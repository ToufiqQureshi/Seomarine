# Team and workspace membership: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Invites teammates, lists roles/seats and transfers workspace ownership.

## Decisions to preserve

- An invite can send email and create a workspace membership: keep plan, permission and rate limits before side effects. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Only the owner can transfer ownership; never remove the current owner through ordinary member removal. _Why:_ These actions affect a customer account, site or external service and must follow an explicit user action with a clear result.
- Workspace switch and member data must be authorized server-side, not trusted from UI state. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.

## Verification pointers

- `teamApi.test.ts`
