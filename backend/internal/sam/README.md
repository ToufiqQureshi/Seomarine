# SAM

SAM is the in-app SEO agent: a project-bound chat that can call the same
tools Uplumo exposes over MCP. This package owns the session registry, the
conversation routes, the agent loop and its tool surface.

## Flow of one turn

1. `POST .../sam/sessions/chat` validates the body, loads the caller's own
   active session (`ContextLoader`), claims the session (one turn at a time) and
   stores the user message.
2. `Agent.Run` opens the event stream and gates the turn: no credits, no
   membership, or credits that cannot be checked (it fails closed) means a
   canned reply, with no provider call.
3. The system prompt is rebuilt fresh: identity, the project's shared memory
   (`projectcontext`), the skill catalog, and the rolling summary if the chat has
   one. Context written by another session, the settings UI or an MCP client is
   therefore current on every turn.
4. Up to 40 steps: stream one OpenRouter completion, run the tool calls it asked
   for, feed the results back. Every event goes out as an AI SDK UI message
   stream chunk and is folded into the assistant message that gets stored.
5. The assistant message is stored, unbilled spend is charged, the session is
   named from its first message (or touched), and a long chat is compacted.

The reply streams as server-sent events in the AI SDK UI message stream format
(`x-vercel-ai-ui-message-stream: v1`), so the React client keeps using `useChat`.
A test parses the real output with the SDK's own parser.

## Endpoints

All are `POST /api/v1/projects/{projectId}/sam/...`, need a session and project
membership, and take a strict JSON body. Spec: `backend/api/sam.yaml`.

- `sessions/list`, `sessions/create`, `sessions/archive`: the registry.
- `sessions/messages`: the stored transcript, oldest first.
- `sessions/chat`: send a message, stream the reply. `409` while a reply is
  running or when the message id was already used.
- `sessions/rewind`: delete a message and everything after it. Stops a running
  turn first, otherwise its loop would store a reply to a message that is gone.
- `access-setup-status` lives in `httpapi`.

## Tables

- `sam_sessions` (legacy table): the registry row. `updated_at` is ISO text, so
  title and touch updates write the same format.
- `go_sam_messages`: the transcript. `parts` is jsonb in AI SDK `UIMessage` part
  shape, so history and live replies render identically.
- `go_sam_session_summaries`: one rolling summary per session.

## Env vars

- `OPENROUTER_API_KEY`: Uplumo's own key, set in Railway. Customers never
  supply one. Without it the chat routes answer 503 and the app shows "SAM is
  unavailable right now".
- `OPENROUTER_MODEL`: optional. Default `openai/gpt-5.6-luna` at reasoning
  effort `max`.

## Rules that must never break

- **The project is bound server-side.** `projectId` is removed from every tool
  schema the model sees and injected at call time. A `projectId` the model
  supplies anyway is overwritten; a tool that takes none never receives one.
- Sessions are loaded by `(session, project, user)` and a miss is "not found",
  never "forbidden". Archived sessions and projects are not loadable.
- The allowlist (`samMCPTools`) is explicit. A test builds it against the real
  MCP registry, so a renamed or removed tool fails the build instead of silently
  vanishing from SAM. `list_projects`, `create_project`, report tools and audit
  history/delete are deliberately absent.
- Tool results are capped (`capToolOutput`) before the model sees them; every
  result is re-sent on each later step of its turn.
- Reasoning is never sent back to the provider.
- Every model call (each step, and the history summary) first holds its
  ceiling from the workspace's credits (`openrouter.MaxCostUSD`: the full
  output budget plus the input, at the expensive fallback prices) and settles
  the real cost when it ends, on a context that outlives the request. Parallel
  turns cannot together spend past the balance. A stopped turn still consumed
  what it consumed. The refusal replies are free; running out, the workspace's
  daily limit and a global pause each have their own reply.
- Site reading goes through the audit `Guard` (private, loopback and link-local
  addresses blocked at dial time); a redirect is followed one hop, after the
  destination passes the same validation.

## Edge cases

- A message containing a NUL byte would make Postgres reject the whole jsonb
  insert and lose the reply; `stripNULEscapes` removes them before the write.
- A tool call with invalid JSON arguments gets `{}` as its stream input and an
  error result, so the model can recover and the stream stays valid.
- A tool that never returned (client left mid-tool) is stored as `output-error`,
  never left "running".
- Provider failures reach the client as a generic error chunk; provider text is
  logged, not shown.
- A removed member keeps no access: membership is read on every turn, and
  a missing member row is refused.

## Production limits

- The one-turn-per-session lock is in memory, so it assumes one server
  instance. Running several replicas needs a Redis lock before it is safe.
- Compaction runs between turns only. A single turn is bounded by 40 steps, 16k
  output tokens per step and the tool output cap, not by a mid-turn guard.

## Deliberate differences from the TypeScript version

- **Transport.** HTTP + SSE replaces the Cloudflare Durable Object websocket.
  Stopping the reply is aborting the request. There is no reconnect or
  "recovering" state because no process holds a turn that can be reset.
- **Credits.** Every turn is gated and charged through the billing spend
  guard (`billing.AIMeter`, feature `ai_assistant`). Credits are checked again
  before every step, so a turn stops when they run out or the daily limit is
  reached, and a turn stops at `maxTurnUSD` ($1) of provider cost. A step
  whose cost OpenRouter did not report is charged an estimate
  (`openrouter.Usage.BillableUSD`). The legacy turn failed open on a billing
  outage; this one fails closed. Uplumo is SaaS: there is no
  bring-your-own-key or unmetered mode.
- **Telemetry.** The `sam:turn` PostHog events are not ported; failures are
  logged with `slog`.
- **Transcripts.** Existing Durable Object transcripts are not migrated.
- **Model routing.** The MiniMax-specific provider pinning is not ported; any
  `OPENROUTER_MODEL` is called with default routing.
- **Dev tools.** The dev-only "clear history" button is gone; rewind covers it.
- **Skills and fact sheet.** `skills/*.md` and `product_fact_sheet.md` are copies
  of files outside the Go module. `TestEmbeddedCopiesMatchTheirSources` fails
  when they drift.
