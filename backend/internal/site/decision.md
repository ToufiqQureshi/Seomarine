# Public website, pricing and blog: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Serves Uplumo marketing pages, pricing, blog posts, robots.txt, sitemap.xml and static assets from Go.

## Decisions to preserve

- Do not publish malformed/unreviewed content or expose drafts in sitemap. _Why:_ These actions affect a customer account, site or external service and must follow an explicit user action with a clear result.
- Escape post fields and metadata; keep canonical URLs correct and URLs derived from trusted config. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.
- Pricing must be sourced from the current plan catalog. _Why:_ A single server-owned source keeps UI access, checkout and limits aligned even if client state is stale or modified.

## Dependencies

Embedded/site templates, markdown blog content, plan catalog, static assets.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
