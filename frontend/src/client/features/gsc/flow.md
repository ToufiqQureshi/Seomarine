# Search Console connection prompts: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. When the app detects that Search Console needs attention, the project UI opens `GscReEngagementModal`.
2. The user can follow the reconnection path; OAuth and property state are handled by integration screens and Go GSC/Google services.
3. After reconnecting, Search Performance queries the selected property.

## Implementation map

- `GscReEngagementModal.tsx`
- `frontend/src/client/features/integrations`
- `backend/internal/gsc`
- `backend/internal/google`

## API and service dependencies

Server packages: `backend/internal/gsc` and `backend/internal/google`; connection controls are in `frontend/src/client/features/integrations`.

## Cost, security and failure boundaries

- The modal is not the source of OAuth token or property state; keep ownership in `integrations` and Go `google`/`gsc`.
- Do not imply a property is connected until the server confirms it.
- Google API reads are quota-limited even though they do not use paid SEO credits.
