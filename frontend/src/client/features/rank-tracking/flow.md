# Rank tracking: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User creates a tracker, selects keywords and a check schedule, then saves the configuration.
2. A manual check or scheduled worker creates a run; the UI polls its status and displays results/history.
3. Go validates and estimates cost, reserves credits, enqueues bounded tasks, persists each result and settles actual spend.
4. Metrics refresh is a separate explicit action and must show its credit cost.

## Implementation map

- `RankTrackingConfigModal.tsx`
- `rankTrackingApi.ts`
- `useRankCheckTrigger.ts`
- `useRankRunPolling.ts`
- `backend/internal/ranktracking`

## API and service dependencies

Go package: `backend/internal/ranktracking`; jobs use the shared queue, provider costs use billing/spend guards.

## Cost, security and failure boundaries

- Rank checks consume provider credits; require a visible estimate/ceiling and enforce it again on the server.
- Scheduled checks must stop or pause on limits; resuming must not duplicate tasks or charges.
- A partial provider error remains a partial run, not fabricated position data.
- Opening tracker screens must not start a check.
