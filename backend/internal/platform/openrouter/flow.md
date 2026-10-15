# OpenRouter streaming transport: flow

> Internal guide for developers and coding agents.

## Current behavior

Caller supplies bounded messages/tools and mandatory max tokens; client streams SSE under request context; it returns accumulated text/tool calls/usage including partial output on stream error. It never executes tools or retries.

## Dependencies

OpenRouter API, shared HTTP client and caller-owned billing guard.

## Relevant code

- `openrouter.go`
- `openrouter_test.go`
