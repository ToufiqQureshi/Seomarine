# Reports and share links: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User opens Reports; queries load existing report records/templates.
2. Creating or editing a template/report sends explicit mutations through `reportApi.ts`.
3. Go scopes records to the project, builds the rendered view and persists share settings when enabled.
4. A recipient opens a share URL and receives only the report content allowed by its share state.

## Implementation map

- `ReportsList.tsx`
- `ReportTemplateForm.tsx`
- `ReportViewer.tsx`
- `reportApi.ts`
- `backend/internal/reports/share.go`

## API and service dependencies

Go package: `backend/internal/reports`; branding is read from `backend/internal/branding` where applicable.

## Manual reports and client work logs

The report composer supports a standard report and a client work log. A standard report takes a title, summary and plain-text body. A work log takes a title, summary, reporting month and completed work, with optional outcomes, evidence links and next steps. The browser limits the title to 120 characters, summary to 2,500, standard body to 60,000, and total work-log details to 20,000. It permits at most 10 credential-free HTTP(S) evidence URLs and escapes every field into a self-contained HTML document before saving it through `reportApi.ts` as `Manual` or `Manual work log`. This path does not call an AI or paid data provider.

Sharing a work log is a separate opt-in action after the save succeeds. If share-link creation fails, the report remains saved and the UI reports that only the link failed. Report list queries are invalidated and the user is sent to the saved report.

## Cost, security and failure boundaries

- Public sharing is opt-in; never expose a report through an unguessable-but-unchecked ID alone.
- Keep organization/project authorization on private routes and revocation effective for shared links.
- Report generation should use stored data unless a clearly labeled refresh action is requested.
