# Backlinks Go migration

| Area           | Status      | Notes                                                                                         |
| -------------- | ----------- | --------------------------------------------------------------------------------------------- |
| Go API         | In progress | Overview, backlink rows, referring domains and top pages are in `backend/internal/backlinks`. |
| React callers  | In progress | Backlinks page and search-tab previews call the Go endpoints with Zod response validation.    |
| Legacy service | Retained    | MCP backlinks tools still import it; remove after the MCP migration.                          |
| Database       | Not needed  | Backlinks are provider-backed and this port adds no tables.                                   |

## Deliberate differences from TypeScript

- The Go handler rejects unknown JSON fields and range minima above their maxima before any provider task is billed.
- The HTTP surface is session and project protected, and checks the paid plan before the cache/provider path. With billing absent it remains open, matching AI search.
- Redis cache keys use the normalized target, organization, page, filters and spam option. Redis failures are soft.
- Referring domains remain unfiltered by spam because the React UI explicitly requests the raw aggregate set.

## Validation

The port keeps the React JSON field names and nullable values. Provider requests use the shared DataForSEO client with retry disabled so task costs are attributed to the project organization and paid tasks are not replayed.

The feature has no repository, so repository tenant-isolation tests do not apply. Project membership and organization-attributed billing are verified by the HTTP API integration test when Postgres and Redis are configured.
