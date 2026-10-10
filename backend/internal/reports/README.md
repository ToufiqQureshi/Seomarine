# Reports

Reports are project-owned HTML documents produced by agents. Keep their body
out of list queries: the HTML cap exists because report documents are copied
through request buffers and database calls, while ordinary lists need only
metadata and a short summary. Every private lookup is scoped by both project
and report ID; a guessed child ID must not cross project boundaries.

Templates are short project briefs, not report content. Report updates replace
the document in place and have no version history, so validate the complete
document and storage ceilings before writing. Share tokens are bearer
capabilities with 192 bits of random entropy; only hosted deployments issue
them, and revoking a token must make the public read fail immediately.

The tables predate Go and remain in the existing Drizzle schema until schema
cutover. Go uses plain SQL against `reports` and `report_templates`; it does not
add or rename columns in this port.

MCP clients use the Go reports service for report read/write, public sharing,
and template tools. Every operation first authorizes the requested project;
repository reads and writes then scope records to that project. Public links
are enabled only when `AUTH_MODE=hosted`, and share tokens are never included
in report lists or ordinary report metadata. API clients use the same Go
service through the authenticated `/api/v1/projects/{projectId}/reports/*`
routes. Go also serves the public report document at `/s/{token}/raw` with a
strict sandbox CSP; the wrapper page and social image remain on their existing
frontend routes. The legacy TypeScript tools remain registered but are shadowed by the
Go registry while the dispatcher runs; remove their implementation only after
the external MCP contract suite passes.
