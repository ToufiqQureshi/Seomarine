# Analytics visitor geography: decisions

> Internal guide for developers and coding agents. Update with code changes.

## Responsibility

Resolves a public IP to an ISO country code using the embedded DB-IP country database.

## Decisions to preserve

- Never persist or log the raw IP as a side effect of lookup. _Why:_ Collecting or retaining less identifying data reduces privacy exposure if logs, storage or exports are accessed improperly.
- Private, invalid or unmapped IPs return no country; do not infer a location. _Why:_ A silent fallback can target the wrong market or metric and produce misleading or wasted research.
- Keep `ATTRIBUTION.md` and the DB-IP licence attribution intact. _Why:_ Preserving provenance and required attribution keeps the shipped data legally usable and reproducible.

## Cost and risk boundary

Embedded offline MMDB; called by Analytics ingestion.
