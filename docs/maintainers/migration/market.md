# Market data Go migration

| Area | Status | Notes |
| --- | --- | --- |
| Country and language tables | Done | Generated from `src/shared/keyword-locations.ts`; 143 locations, 128 languages, 20 multilingual overrides. |
| Market resolution | Done | `backend/internal/platform/market` ports default, Labs, language and provider selection behavior. |
| Domain server-side resolution | Pending | Domain requests still receive resolved markets from React. Wire a project-scoped DB lookup before removing `domainMarket.ts`. |
| Keywords, SERP locations, rank tracking | Pending | No Go API or job workflow yet. |

The generated table is an interim migration asset. Once Go owns market configuration, make the Go data the source of truth and generate any frontend picker data from it.
