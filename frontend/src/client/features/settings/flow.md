# Workspace and account settings: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. Settings route selects a settings panel.
2. Each panel uses its feature API: API keys use Better Auth API-key endpoints, branding uses `/api/v1/branding`, and crawler access uses crawler-access routes.
3. The backend independently checks session, role, plan and project scope for each mutation.
4. Sensitive values are shown once or redacted after save.

## Implementation map

- `ApiKeySettings.tsx`
- `BrandingSettings.tsx`
- `CrawlerAccessSettings.tsx`
- `backend/internal/apikeys`
- `backend/internal/branding`
- `backend/internal/crawleraccess`

## API and service dependencies

Go packages: `apikeys`, `branding`, `crawleraccess`.

## Cost, security and failure boundaries

- Settings is a navigation shell, not a shared authorization boundary; each backend feature enforces its own permissions.
- Plaintext API keys and crawler credentials must be shown only when created, then never returned by list endpoints.
- Do not combine unrelated settings saves into a mutation that can partially corrupt workspace configuration.
