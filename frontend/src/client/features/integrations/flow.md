# Google integrations: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User starts Google OAuth from the connection card; `googleOAuthApi.ts` starts the selected provider flow.
2. The callback returns to Uplumo; Go validates state/code and stores encrypted tokens.
3. The UI lists accessible GA4/GSC properties, lets a member select one for a project, and shows connection state.
4. Removal previews impact first; owner-confirmed removal deletes the account grant and affected connections.

## Implementation map

- `GoogleConnectionCard.tsx`
- `googleOAuthApi.ts`
- `ga4ConnectionsApi.ts`
- `gscApi.ts`
- `backend/internal/google`
- `backend/internal/ga4`
- `backend/internal/gsc`

## API and service dependencies

Go packages: `google`, `ga4`, `gsc`; OAuth tokens are encrypted server-side.

## Cost, security and failure boundaries

- OAuth state, redirect allowlists, token encryption and refresh-once behavior protect account access.
- Request only the read-only scopes the product needs; never expose access/refresh tokens to React.
- Property listing and reporting consume Google quota, so use bounded requests and avoid refetch loops.
- Account removal is destructive; preview impact and scope it to the owner.
