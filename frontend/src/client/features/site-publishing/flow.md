# WordPress publishing: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User connects a WordPress site and saves the connection.
2. For a draft, the UI loads publication state and previews the selected schema blocks.
3. User explicitly clicks publish; `sitePublishingApi.ts` sends the draft ID to Go.
4. Go verifies project access, role, plan, connection and draft safety, then posts to WordPress and records the result.

## Implementation map

- `PublishToSite.tsx`
- `sitePublishingApi.ts`
- `backend/internal/sitepublish/handler.go`
- `backend/internal/sitepublish/wordpress.go`

## API and service dependencies

Go package: `backend/internal/sitepublish`; draft source is `backend/internal/contenteditor`.

## Cost, security and failure boundaries

- Publishing is a customer-site side effect: never publish from draft autosave, preview or AI generation.
- Retain explicit owner/admin permissions and plan checks.
- Sanitize outbound HTML/schema and do not leak WordPress credentials in logs or errors.
- A timeout after WordPress accepts a post must not cause duplicate posts on blind retry.
