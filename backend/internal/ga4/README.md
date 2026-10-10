# GA4 reports

This package serves read-only Google Analytics 4 reports for a project. The
handler authorizes the session and project first, the service applies report
rules, and the repository reads the selected property and OAuth account from
the existing `ga4_connections` row. Google tokens are always requested for
the connecting user, the `google-analytics` grant, and that grant's account.

The Go endpoint is `POST /api/v1/projects/{projectId}/ga4/reports/run`.
It returns the existing report JSON shape, including source and date metadata,
rows, pagination, quota, warnings, optional previous-period comparison, and
report-specific diagnostics/activity. Report filters and dimensions stay
explicit because GA4 rejects incompatible combinations and may bill failed
requests.

## Rules and edge cases

- The connection table is still owned by the legacy GA4 setup flow; this read
  path does not create or alter it. The project-access middleware is the
  authorization boundary for that lookup.
- A report defaults to organic search and the last 28 complete days in the
  property's IANA timezone. Future end dates clamp to yesterday in that zone;
  partial, malformed, or reversed date ranges fail before contacting Google.
- Page size is 1–1,000, offset is non-negative, and provider responses are
  checked against the exact requested dimension and metric headers. Invalid
  rows and non-finite values fail closed. Restricted metrics become `null` and
  mark the report as limited.
- Comparison is available only for event key events, channel-group acquisition,
  device audience, and new-versus-returning audience. Complete report reads are
  capped at the same 1,000 rows used by the legacy service; a requested page
  beyond that buffer is fetched separately.
- GA4 API, OAuth grant, quota, and malformed-report failures use stable
  `ga4_*` error codes. The shared Google API client only retains Google's
  allowlisted `SERVICE_DISABLED` reason and numeric `Retry-After` value; it
  never includes provider error text in logs or responses.
- Property listing, connection setup/removal, the organic overview and
  measurement-health reports, the MCP wrappers, and the React call-site switch
  remain on the legacy implementation for later roadmap items. No legacy code
  is deleted by this API port.

## Environment

Uses the existing Postgres connection and Google OAuth credentials. No new
environment variable or database migration is needed for report reads.
