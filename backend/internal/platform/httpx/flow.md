# Shared HTTP helpers: flow

> Internal guide for developers and coding agents.

## Current behavior

Handlers call the shared decoders/writers; middleware assigns a validated request ID, logs status/duration and recovers panics with a generic response.

## Dependencies

Go net/http and shared handlers.

## Relevant code

- `context.go`
- `httpx_test.go`
