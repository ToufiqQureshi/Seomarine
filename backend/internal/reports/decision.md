# Reports and share links: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Stores project reports/templates, renders HTML and manages optional public share state.

## Decisions to preserve

- Keep report body out of list queries and enforce content size limits. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.
- Public links require explicit share state and revocation; never expose private IDs as a substitute for auth. _Why:_ Report content can contain client data, so the public route must only resolve an explicitly shared token and revocation must take effect.
- Render user/agent HTML safely and preserve ownership on update/delete. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.
- Manual content uses the existing project-scoped report API and remains separate from model generation. Keep request ownership and report size validation in the server path even when the browser validates first. _Why:_ Browser checks can be bypassed, while the API is the durable authorization and storage boundary.
- Saving a report and creating its public share link remain separate mutations. _Why:_ A public URL exposes customer content and must only exist after a deliberate sharing choice.

## Dependencies

Postgres report/template data; optional branding; project auth.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
