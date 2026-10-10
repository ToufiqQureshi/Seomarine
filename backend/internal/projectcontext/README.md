# Project context

Project context is the shared memory used by the settings UI, MCP agents and
SAM. The Go package owns the existing Postgres tables and applies an entire
update batch in one transaction while holding the active project row lock. This
keeps caps, canonicalization and uniqueness checks consistent across writers.

Typed and custom prose is limited to 4,000 UTF-16 characters. A project can
store up to 20 custom sections, 100 competitors, and 100 key pages. Competitor
domains and key page URLs are canonicalized before writes; omitted competitor
notes and key page roles/notes retain stored values on upsert. Research log
entries are server dated, retained for 90 days, and reads return the newest 20.

Authenticated routes are `POST /api/v1/projects/{projectId}/context/get` and
`POST /api/v1/projects/{projectId}/context/update`. Routes require a signed-in
project member. MCP and the HTTP API call the same
service so agents and people see the same memory.
