# articletracking

Durable opt-in and stored-result contract for tracking an article after a
successful site publication.

## API

- `POST /api/v1/projects/{projectId}/content/drafts/{draftId}/tracking/opt-in`
  with `{"targetKeyword":"..."}`. Requires session and project-access
  middleware. Organization scope comes from the authenticated project context.
  The repository checks the matching `go_site_publications` row has
  `remote_status='publish'` and uses its `remote_url`; draft status cannot be
  supplied by the browser. Repeated opt-in for the same draft returns the same
  row. Draft deletion cascades to tracking and AI history.
- `POST .../tracking/get` with `{}` returns the tracking row and latest stored
  AI result. It does not call a provider.

## Required application wiring

1. Mount these endpoints alongside content-editor routes with the same session
   and project-access middleware.
2. After opt-in, call the existing `ranktracking.Service` with the owning
   project's market and published URL hostname. Reuse the existing project
   config when present, add the target keyword through `AddKeywords` (so plan
   limits and keyword caps remain authoritative), then persist its config ID
   with `SetRankConfig`. Only link a config that belongs to the same project
   and organization. The existing rank scheduler owns Google checks and its
   snapshots; load those snapshots through ranktracking's read service.
3. Add a periodic job that consumes `DueAI`, invokes `aisearch.Service` for each
   returned prompt using the organization's existing plan/credit controls,
   stores only a sanitized provider result through `SaveAIResult`, and advances
   the next run. Use a bounded batch and an explicit schedule; failed provider
   outcomes should be stored as safe status data. Do not run provider requests
   from `Get` or page-load handlers.
4. Add frontend opt-in after successful publishing, show current rank and AI
   mention from the stored data, and allow tracking to be disabled if product
   requirements call for it. No tracking is implicit before opt-in.

The periodic AI runner is intentionally not embedded in HTTP request handling:
`aisearch.Service` is currently an on-demand service with Redis caching, not a
durable scheduled tracker. Wiring a loop here without registering it with the
application lifecycle would silently leave tracking stale.
