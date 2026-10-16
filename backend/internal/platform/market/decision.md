# Market registry: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Maps canonical countries, languages, DataForSEO locations and supported keyword markets.

## Decisions to preserve

- Canonical market resolution must be deterministic and tested. _Why:_ A silent fallback can target the wrong market or metric and produce misleading or wasted research.
- Never silently substitute a different country/location; that can waste spend and mislead customers. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Generated data provenance/update steps belong in README and must not ship a partial registry. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Dependencies

Versioned bundled market/location data; DataForSEO market support.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
