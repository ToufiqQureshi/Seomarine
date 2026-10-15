# Public website, pricing and blog: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** GET /, /pricing, /blog, /blog/{slug}, /robots.txt, /sitemap.xml, assets
- **Source:** `site.go`, `blog.go`

## Request or job flow

HTTP router calls a site handler; content is parsed and validated; templates render escaped metadata/body and canonical URLs; sitemap lists public pages/posts; assets are served with cache validators.

## Dependencies and cost boundary

Embedded/site templates, markdown blog content, plan catalog, static assets.

## Failure, security and data invariants

- Do not publish malformed/unreviewed content or expose drafts in sitemap.
- Escape post fields and metadata; keep canonical URLs correct and URLs derived from trusted config.
- Pricing must be sourced from the current plan catalog.
