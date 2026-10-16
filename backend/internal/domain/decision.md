# Domain overview and research: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Validates target scopes and returns organic estimates, ranked terms, pages, competitors and suggestions.

## Decisions to preserve

- Validate target, market and filter budget before spend; failed provider tasks may still be billed. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Keep scope semantics explicit: domain, subdomain, subfolder and exact URL do not support identical aggregates. _Why:_ A silent fallback can target the wrong market or metric and produce misleading or wasted research.
- Escape user filters for SQL/provider syntax and cap response size. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.

## Dependencies

DataForSEO paid endpoints, project market and plan checks.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
