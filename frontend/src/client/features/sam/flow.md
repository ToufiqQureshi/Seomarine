# SAM assistant: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User opens SAM and selects/creates a project session.
2. Messages stream through `samApi.ts`; Go loads bounded project memory/history and invokes the model.
3. If the model requests a tool, Go authorizes the same project, runs a registered tool, and returns its result to the model.
4. The turn persists messages and usage; the UI supports history, rewind and archive actions.

## Implementation map

- `SamConversation.tsx`
- `samApi.ts`
- `useSamSessions.ts`
- `backend/internal/sam/agent.go`
- `backend/internal/sam/chat.go`
- `backend/internal/sam/tools.go`

## API and service dependencies

Go package: `backend/internal/sam`; uses the configured model provider, billing, project context and registered Go services.

## Cost, security and failure boundaries

- Model/tool usage can incur variable cost; enforce per-turn/daily limits, reserve/settle credits and stop at the ceiling.
- Tool arguments cannot choose another project or bypass project membership.
- Provider errors must preserve already-produced partial output while charging actual usage only.
- Avoid refetching or replaying a streamed POST automatically.
