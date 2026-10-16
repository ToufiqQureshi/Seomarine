# Uplumo Analytics: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. A project member opens Analytics and requests the site snippet through `analyticsApi.ts`.
2. The Go analytics service creates or returns one site key per project; the UI shows the tracker snippet.
3. The customer site loads `/t.js`; pageviews and opted-in custom events post to the public `/collect` endpoint.
4. The Analytics page loads summary/country data and displays traffic, sources, devices, events and geography.

## Implementation map

- `AnalyticsPage.tsx`
- `TrackingCodeCard.tsx`
- `analyticsApi.ts`
- `backend/internal/httpapi/tracker.js`
- `backend/internal/analytics/service.go`

## API and service dependencies

Go package: `backend/internal/analytics`; ingestion route is mounted in `backend/internal/httpapi`.

## Cost, security and failure boundaries

- Do not add cookies or persist raw IP addresses; visitor IDs are daily salted hashes.
- Collection is public by design, so preserve bot/domain checks and per-IP/per-site rate limits.
- Custom events must use the same validation and rate limits as pageviews.
- This feature does not call paid SEO providers.
