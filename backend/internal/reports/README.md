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
