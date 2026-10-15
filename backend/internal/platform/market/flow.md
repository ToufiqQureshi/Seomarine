# Market registry: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** No route; used before provider requests and by market selectors.
- **Source:** `market.go and generated registry data`

## Request or job flow

Feature supplies country/language/location; registry normalizes and checks provider support; unsupported combinations are rejected before a paid request.

## Dependencies and cost boundary

Versioned bundled market/location data; DataForSEO market support.

## Failure, security and data invariants

- Canonical market resolution must be deterministic and tested.
- Never silently substitute a different country/location; that can waste spend and mislead customers.
- Generated data provenance/update steps belong in README and must not ship a partial registry.
