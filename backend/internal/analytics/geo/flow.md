# Analytics visitor geography: flow

> Internal guide for developers and coding agents.

## Current behavior

New opens the embedded database once at startup; Country rejects invalid/private addresses and returns a country only when a public address is in the bundled database.

## Dependencies

Embedded offline MMDB; called by Analytics ingestion.

## Relevant code

- `geo.go`
- `geo_test.go`
- `ATTRIBUTION.md`
