# AI agent setup: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Why the feature is structured this way

Helps a user configure an external AI agent to connect to Uplumo MCP tools.

## Decisions to preserve

- **Keep setup copy and generated prompt aligned with the server URL and supported auth methods** _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- **Never show or persist a user API key in the setup instructions** — key creation and revocation live in Settings/API keys. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- **Treat agent instructions as product-visible content** — avoid promising tools that are not registered on the Go MCP server. _Why:_ Keeping product meaning aligned with supported behavior avoids misleading customers and incorrect agent responses.

## Verification pointers

- `agentSetupPrompt.test.ts`
