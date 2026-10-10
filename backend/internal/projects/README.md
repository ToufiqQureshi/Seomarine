# Projects

Go serves the organization-scoped project API at these routes:

- `POST /api/v1/projects/list` — list active projects and create the reserved
  `Default` project only when none exist.
- `POST /api/v1/projects/create` — create a project (organization owner/admin).
- `POST /api/v1/projects/archived` and `/restore` — archived-project management
  (restore requires owner/admin).
- `POST /api/v1/projects/{projectId}/get`, `/update`, and `/archive` — project
  details, member-editable settings and owner/admin archive.

Project domains use the same IDNA and public-suffix checks as backlink target
validation. Market pairs are checked against the shared DataForSEO market
registry. Archive operations lock the organization row and recheck the active
project count in the same transaction, so concurrent requests cannot archive
the final project. Restore conflicts on the reserved active `Default` project
follow the existing unique index.

The React pages are not switched here; the API handlers are ready for the
separate client cutover.
