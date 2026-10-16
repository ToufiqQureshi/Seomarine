# Workspace and account settings: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Groups API-key, report-branding and crawler-access settings under a single settings experience.

## Decisions to preserve

- Settings is a navigation shell, not a shared authorization boundary; each backend feature enforces its own permissions. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.
- Plaintext API keys and crawler credentials must be shown only when created, then never returned by list endpoints. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Do not combine unrelated settings saves into a mutation that can partially corrupt workspace configuration. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Verification pointers

- Verify the listed source files and the feature route manually when behavior changes.
