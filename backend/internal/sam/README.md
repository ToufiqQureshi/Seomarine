# SAM sessions

## Status

Go now serves the SAM session registry: project-scoped list, create and soft
archive routes. The code reads the existing `sam_sessions` table so session IDs
and transcripts remain shared with the current chat runtime. No new table or
migration is needed.

The AI chat loop, streaming transcript storage, tools, skills, context blocks,
credit metering and telemetry remain in the TypeScript Cloudflare Durable
Object. The React client is not switched to these routes yet.

## Behavior

- Every route requires a signed-in project member. Lists only include the
  caller's active sessions, ordered by `updated_at DESC, id DESC`.
- Creating a session inserts the same `New chat` registry row used by the
  current client.
- Archiving is a soft update scoped by session ID, project ID and user ID.
  Archived sessions disappear from the list; their transcript is retained.
- Requests are POST JSON objects and reject unknown fields.

Routes and request contracts are in `backend/api/sam.yaml`.

## Deliberate differences

The legacy list/create server functions also emit product analytics events.
These Go routes do not emit those events. The chat connection still uses the
existing authorized Durable Object path until the agent runtime and UI are
ported.
