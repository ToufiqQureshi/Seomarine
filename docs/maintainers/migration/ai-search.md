# ai-search

Brand Lookup and Prompt Explorer. ROADMAP 2.4a.

## Status

| Piece                                                                | State       | Notes                                                                                                          |
| -------------------------------------------------------------------- | ----------- | -------------------------------------------------------------------------------------------------------------- |
| `backend/internal/aisearch`                                          | done        | Handler, service, provider, shaping, tests. See its README.                                                    |
| `platform/dataforseo` envelope + usage recorder                      | done        | `Results` classifies task failures; `UsageRecorder` writes `go_dataforseo_usage`.                              |
| `auth.AuthorizeProject` returns the project's organization           | done        | Needed to bill the right organization.                                                                         |
| `src/client/features/ai-search/aiSearchApi.ts`                       | done        | The two pages call the Go API through `apiRequest`.                                                            |
| `src/serverFunctions/ai-search.ts`, `src/server/features/ai-search/` | deleted     | Replaced by the Go package.                                                                                    |
| `scripts/brand-lookup-cost-profile.ts`                               | deleted     | Imported the deleted service; real per-call cost is now in `go_dataforseo_usage`.                              |
| `src/server/lib/dataforseo/{ai,llm-models,pricing,client}.ts`        | **blocked** | Shared metered client; still used by the legacy credit system. Goes with `billing` and the remaining features. |
| `src/shared/{researchScope,targetDetection,safe-url}.ts`             | **blocked** | Still imported by the client and by other features.                                                            |
| GDPR erasure of cached prompts                                       | **open**    | `src/server/gdpr/storage-erasure.ts` clears the old R2 namespace only; see the README's Cache section.         |

## Differences from the TypeScript behavior

- Hosted gating is the Go billing plan (Razorpay), not Autumn credits. There is
  no per-call credit charge; spend is only recorded.
- Cache moved from R2 to Redis (per-organization key prefix).
- Brand lookup stops at the first billing failure instead of finishing the
  other platform's calls first.
- `Intl.DisplayNames` country names come from `golang.org/x/text`, so a few
  territory names may differ in wording.
