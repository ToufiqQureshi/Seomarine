# Rank tracking

Configures keywords, devices/locations and manual or scheduled rank checks with history.

Weekly is the default. The setup shows the estimated monthly credits before
save, counts desktop plus mobile as two checks, and leaves discovered keyword
suggestions unselected so saving a domain cannot silently add paid work.

Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for the rules that protect user data, provider spend and customer trust.

## Code map

- `RankTrackingConfigModal.tsx`
- `rankTrackingApi.ts`
- `useRankCheckTrigger.ts`
- `useRankRunPolling.ts`
- `backend/internal/ranktracking`
