# Shared random IDs: flow

> Internal guide for developers and coding agents.

## Current behavior

A repository/service requests a new ID; cryptographic randomness fills bytes and the package sets UUID v4/variant bits before formatting.

## Dependencies

Go `crypto/rand`; no external provider or stored state.

## Relevant code

- `ids.go`
- `ids_test.go`
