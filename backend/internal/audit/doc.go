// Package audit ports the legacy TypeScript site-audit engine to Go: the
// SSRF-guarded crawler, the HTML page analyzer, the per-page and cross-page
// issue checks, Lighthouse sampling through DataForSEO, robots.txt/sitemap
// discovery, Redis-backed crawl progress, and the Postgres persistence and API
// for audits, pages, issues and Lighthouse results.
//
// The package keeps the exact JSON contract the React audit pages already use,
// so switching a caller only changes its transport (server function -> Go API).
//
// See README.md for the behavior inventory, the rules that must never break and
// every deliberate difference from the TypeScript implementation.
package audit
