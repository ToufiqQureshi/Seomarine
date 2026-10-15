# Team and workspace membership: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. Team page queries the active organization and member list.
2. An owner/admin submits an invite; Go validates role, plan seat limit and email rate limits before sending.
3. Invite recipient accepts through the auth invitation flow; membership and active organization are updated server-side.
4. Ownership transfer requires the owner’s explicit confirmation and updates roles atomically.

## Implementation map

- `TeamSettings.tsx`
- `teamApi.ts`
- `organizationApi.ts`
- `backend/internal/team`
- `backend/internal/auth/ba_organization.go`

## API and service dependencies

Go packages: `team` and `auth`; transactional mail uses `backend/internal/email`.

## Cost, security and failure boundaries

- An invite can send email and create a workspace membership: keep plan, permission and rate limits before side effects.
- Only the owner can transfer ownership; never remove the current owner through ordinary member removal.
- Workspace switch and member data must be authorized server-side, not trusted from UI state.
