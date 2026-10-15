# SAM assistant

Streams project-aware AI answers and tool actions with persisted sessions and credit controls.

Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for the rules that protect user data, provider spend and customer trust.

## Code map

- `SamConversation.tsx`
- `samApi.ts`
- `useSamSessions.ts`
- `backend/internal/sam/agent.go`
- `backend/internal/sam/chat.go`
- `backend/internal/sam/tools.go`
