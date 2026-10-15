# Site audit

Starts an asynchronous crawl and presents technical SEO findings, crawl progress and optional performance data.

Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for the rules that protect user data, provider spend and customer trust.

## Code map

- `launch/useLaunchController.ts`
- `auditApi.ts`
- `AuditDetail.tsx`
- `backend/internal/audit/handler.go`
- `backend/internal/audit/runner.go`

- `launch/AuditChangeGuard.tsx` and `compare.ts` (read-only saved-audit comparison)
- `orphanLinkCandidates.ts` (local, review-only link suggestions)
