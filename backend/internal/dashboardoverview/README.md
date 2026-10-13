# Dashboard overview

This package serves the dashboard's existing audit and backlink snapshot cards.
It performs only database reads; visits never trigger a paid DataForSEO request.
The backlink snapshot is refreshed only by the explicit
`dashboard/backlinks/refresh` call in `internal/dashboard`.

All reads are scoped to the authenticated project and organization. Latest
audits and snapshots use stable newest-first ordering, and stale snapshots are
still returned with an explicit `stale` flag so a provider outage cannot hide
the last known data.
