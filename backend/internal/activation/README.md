# Activation

Go owns dashboard activation reads and state mutations: first click timestamps,
first GA4 card dismissal, per-user setup-step dismissals, and the project
activation snapshot used by the dashboard. Reads authorize through the
project's organization and gather integration, audit, teammate, project-count,
and per-user dismissal state in one database query. Writes preserve
first-occurrence timestamps. The tables are part of migration version 1. The
React dashboard calls these routes.
