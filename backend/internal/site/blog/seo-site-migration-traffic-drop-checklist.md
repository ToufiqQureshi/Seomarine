---
title: SEO Traffic Dropped After a Site Migration? A Recovery Checklist
description: Trace a post-migration traffic loss by URL, redirects, indexability and templates, then prioritise fixes with a practical recovery checklist.
date: 2026-10-15
---

**Short answer:** After a migration, compare old and new URLs page by page before changing more things. Check redirects, response codes, canonical tags, robots directives, internal links, rendered content, and Search Console indexing. Restore broken one-to-one redirects first; then separate technical migration faults from normal ranking changes.

![A practical troubleshooting flow for seo traffic dropped after a site migration? a recovery checklist](/site/blog/seo-site-migration-traffic-drop-checklist.svg)

## Why this problem happens

A redesign, CMS change, URL restructure, domain move, and hosting move create different risks. A dramatic drop often follows missing or inaccurate redirects, blocked pages, changed content, or a launch that combines too many changes to diagnose. Google also needs time to crawl and process a move, so distinguish a short transition from an ongoing fault.

## Check these signals before changing the site

1. Compare Search Console clicks and impressions by page and query before versus after launch. Match the same dates and weekdays; inspect branded and non-branded demand separately.
2. Export the old URL list from the old sitemap, analytics, and crawl data. For each URL, record the new destination and actual HTTP status. Sample the highest-traffic and lead-generating pages first.
3. Inspect a migrated page’s canonical, robots meta, hreflang (if used), rendered text, title, schema, and internal links. Confirm the new page returns 200 and is not blocked by login, firewall, or robots.txt.
4. Review server/CDN logs and Search Console Page Indexing/Crawl Stats for spikes in 404, 5xx, blocked, duplicate, or crawled-not-indexed URLs.

## Fix it in a safe order

### 1. Build an old-to-new map

Map each meaningful old URL to the closest equivalent new URL. Use a permanent server-side redirect when the move is permanent. Preserve URL paths where possible and avoid sending unrelated old pages to the homepage.

### 2. Fix the destination before re-crawling

Make sure the target returns 200, has the intended self-canonical, is indexable, and contains the promised content. Update nav, breadcrumbs, canonicals, hreflang, and XML sitemaps to point directly at final URLs.

### 3. Keep the old domain and redirects live

Do not shut off the old host during the transition. Maintain redirects, verify both HTTP/HTTPS and www/non-www variants, and submit the new sitemap. Use Search Console’s Change of Address tool only when the move meets its requirements.

### 4. Change one layer at a time

If redesign, content rewrite, analytics replacement, and domain move happened together, make a dated change log and triage with URL-level evidence. Fix crawl and redirect errors before rewriting pages based on a traffic chart.

## How to tell the fix worked

Re-crawl the old URL sample and confirm one-hop destinations, then inspect representative new URLs in Search Console. Track old and new properties together until old URLs decline and new URLs gain impressions. Compare leads and revenue too; short-term position swings alone do not prove recovery.

Keep a simple record for each affected URL: the problem, evidence, change, owner, and review date. This is useful when several teams touch the CMS, hosting, analytics, and content. If the same symptom returns, the record helps you spot the shared template or configuration instead of repeating one-off repairs.

## Mistakes that make this worse

Do not blanket-redirect every deleted URL to the new homepage. Do not repeatedly request indexing while a 404, noindex, or wrong canonical remains. Keep the exact launch date and changes in the report.

Before changing a live site, save the current configuration and test the change on a small set of representative URLs. If you are unsure whether an action removes content, changes access, or affects checkout, get the developer or site owner to review it first. Fixing the visible symptom while breaking a related page can cost more than the original search issue.

## FAQ

**How long does SEO recovery take after a migration?**

There is no fixed promise. It depends on site size, crawl frequency, redirect quality, and how many signals changed. Fix detectable errors now and watch URL-level trends over several weeks.

## Related Uplumo guides

- [Website not showing on google](/blog/robots-txt-vs-noindex-pages-still-indexed)
- [Seo audit that finds real problems](/blog/valid-page-returns-404-to-google)
- [Google search console for small businesses](/blog/google-search-console-sitemap-couldnt-fetch)

## Sources

- [Google site moves with URL changes](https://developers.google.com/search/docs/crawling-indexing/site-move-with-url-changes)
- [Google redirects](https://developers.google.com/search/docs/crawling-indexing/301-redirects)
- [Community case: migration without redirect plan](https://www.reddit.com/r/TechSEO/comments/1r58qpy/domain_migration_disaster_98_traffic_drop/)

This guide is educational, not a promise of rankings or a substitute for checking your own site, data, and platform rules. Google and other search systems decide what they crawl, index, and show.
