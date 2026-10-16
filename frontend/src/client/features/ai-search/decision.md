# AI Visibility and prompt research: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Why the feature is structured this way

Brand Lookup compares brand mentions and cited pages across supported answer engines; Prompt Explorer runs a prompt against selected models.

## Decisions to preserve

- **These lookups can incur paid provider cost: keep the server plan/cost gate and input validation before provider calls** _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- **Do not add automatic client retries for a paid POST** — an explicit user retry is a new lookup. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- **Citations are untrusted URLs** — render/link only URLs that the backend has made safe. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.
- **Provider coverage and mention counts are estimates** — the UI must not promise guaranteed ranking or AI inclusion. _Why:_ These provider samples are directional measurements, not a guarantee that a search engine will cite or rank Uplumo customers.
- Read history only after a successful paid Brand Lookup and use stored monthly aggregates; never add a provider call to load the chart. _Why:_ Historical display must not multiply the paid cost of the lookup that collected the data.
- Keep history scoped by organization/project and every result-changing lookup option; show Analytics referrals as a separate measure from citations. _Why:_ Mixed scopes or metrics would produce misleading trends and could expose another project's data.
- Treat the drop notice as an estimate and preserve the existing server-side plan gate. _Why:_ Provider estimates can change and a client-side paywall is not authorization.

## Verification pointers

- `aiSearchApi.test.ts`
