# Activation

Go owns the dashboard's activation state mutations: first click timestamps,
first GA4 card dismissal, and per-user setup-step dismissals. Writes preserve
first-occurrence timestamps and use the existing Drizzle tables; no migration
is required. The React dashboard remains on its current server functions until
the separate client cutover.
