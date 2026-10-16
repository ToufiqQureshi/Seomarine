# AI agent setup: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User opens the agent setup screen and selects the agent they use.
2. The UI shows the matching setup instructions and copyable connection details/prompt; setup text is maintained in `agentSetupPrompt.ts` and `agentSetupPrompt.md`.
3. The external agent later connects to the Go `/mcp` server; authentication and tool execution belong to `backend/internal/mcp`, not this setup UI.

## Implementation map

- `AgentSetup.tsx`
- `AgentSetupPanel.tsx`
- `agentSetupPrompt.ts`
- `agentSetupPrompt.md`

## API and service dependencies

No feature-specific frontend API request is made here; the eventual MCP connection uses `backend/internal/mcp` and API keys from `backend/internal/apikeys`.

## Cost, security and failure boundaries

- Keep setup copy and generated prompt aligned with the server URL and supported auth methods.
- Never show or persist a user API key in the setup instructions; key creation and revocation live in Settings/API keys.
- Treat agent instructions as product-visible content; avoid promising tools that are not registered on the Go MCP server.
