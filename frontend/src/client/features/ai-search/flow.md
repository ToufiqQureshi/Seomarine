# AI Visibility and prompt research: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. The user enters a domain/brand, market and comparison settings in `BrandLookupPage`, or writes a question and chooses models in `PromptExplorerPage`.
2. `aiSearchApi.ts` sends the selected action to `/api/v1/projects/{projectId}/ai-search/{brand-lookup|prompt-explorer}`.
3. Go `aisearch` validates scope, market and model, checks plan access, then uses the DataForSEO AI endpoints; results are shaped for the React cards.
4. The UI renders per-platform status, citations, trends and share-of-voice only from fields returned by the API.

## Implementation map

- `BrandLookupPage.tsx`
- `PromptExplorerPage.tsx`
- `aiSearchApi.ts`
- `backend/internal/aisearch/handler.go`
- `backend/internal/aisearch/service.go`
- `backend/internal/aisearch/provider.go`

## API and service dependencies

Go package: `backend/internal/aisearch`; provider transport: `backend/internal/platform/dataforseo`; plan/credits: `backend/internal/billing` and `backend/internal/plans`.

## Brand Lookup history

After a successful paid Brand Lookup on a paid plan, `BrandLookupPage.tsx` requests saved history with the same project, target, competitors, market and scope. The API reads monthly aggregates from Redis; it makes no additional DataForSEO call. A snapshot is recorded only as part of a complete, data-bearing user-run lookup, at most once per month, and the UI shows up to 13 months. A history read error leaves the current lookup usable. The 30-day AI referral summary comes from Uplumo Analytics and is shown separately; it is site traffic, not visits matched to cited pages. A large month-over-month mention drop is an estimate warning, not a promise of lost rankings.

## Cost, security and failure boundaries

- These lookups can incur paid provider cost: keep the server plan/cost gate and input validation before provider calls.
- Do not add automatic client retries for a paid POST; an explicit user retry is a new lookup.
- Citations are untrusted URLs; render/link only URLs that the backend has made safe.
- Provider coverage and mention counts are estimates; the UI must not promise guaranteed ranking or AI inclusion.
