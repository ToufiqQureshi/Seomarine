# Public website, pricing and blog

Serves Uplumo marketing pages, pricing, blog posts, robots.txt, sitemap.xml and static assets from Go.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `site.go`
- `blog.go`

## API/trigger

GET /, /pricing, /blog, /blog/{slug}, /robots.txt, /sitemap.xml, assets
