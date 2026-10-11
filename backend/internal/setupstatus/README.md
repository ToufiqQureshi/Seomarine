# Setup status

`GET /api/health` gives operators a safe runtime setup report.

In hosted mode it returns only `{"status":"ok"}`. In self-host modes it
reports feature configuration status and a Postgres ping. It never returns
secret values or raw database errors. A failing configuration or database
check sets the overall status to `issues`; warnings for optional features
do not mark the installation unhealthy.

The check keys mirror the self-host preflight: auth, DataForSEO, Google
integrations, AI, JavaScript rendering, and database. Keep the setup report
unauthenticated so a broken deployment can be diagnosed before sign-in.
