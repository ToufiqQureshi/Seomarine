# Content editor: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Creates and edits SEO drafts, content briefs, author profiles, internal-link suggestions, images and publish previews.

## Decisions to preserve

- AI writing/brief calls may spend credits or provider money; keep plan, credit and output-size guards server-side. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Do not auto-save or publish unreviewed generated text to a customer site. _Why:_ These actions affect a customer account, site or external service and must follow an explicit user action with a clear result.
- Uploaded images and generated HTML must be constrained to allowed storage/content types. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.
- Link suggestions use project-owned GSC/audit data and must never cross project boundaries. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.

## Verification pointers

- `ArticlePreview.test.ts`
- `brief-view.test.ts`
