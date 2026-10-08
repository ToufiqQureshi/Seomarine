# Seomarine: feature inventory (do not lose any of these)

Measured on `main` (2026-10-08). Companion to `STATUS.md` (files) and the task files. This file lists WHAT the product does, so nothing is dropped by accident during the TS -> Go migration.

**Rules**

- A row may become `DONE` only when (1) a Go endpoint serves it, (2) the frontend calls the Go endpoint, (3) parity tests pass, (4) the TS server function is deleted. Put the Go route in the "Go endpoint" column.
- Never delete a row. If a feature is intentionally dropped, set `DROPPED` and write the reason and who approved it.
- Add a row when you find a feature this file missed.
- Owner follows `STATUS.md`: Codex or Buffy.

## Totals

| Kind                                                                              | Count | Needs Go?                                     |
| --------------------------------------------------------------------------------- | ----: | --------------------------------------------- |
| Server functions (`createServerFn`, called by the frontend)                       |   108 | YES, each becomes `/api/v1/...`               |
| Server-side routes (webhooks, OAuth callbacks, public report/share, health, auth) |    10 | YES                                           |
| MCP tools                                                                         |    58 | YES (Wave 2)                                  |
| UI pages/layouts (`.tsx` routes)                                                  |    52 | NO, stay React (must keep working against Go) |
| Frontend feature folders (`src/client/features`)                                  |    25 | NO, stay React                                |

## 1. Server functions to port

|   # | Feature         | Function                           | HTTP | TS file (`src/serverFunctions/`) | Owner | Go endpoint | Status |
| --: | --------------- | ---------------------------------- | ---- | -------------------------------- | ----- | ----------- | ------ |
|   1 | ai-search       | `lookupBrand`                      | POST | `ai-search.ts`                   | Buffy |             | TODO   |
|   2 | ai-search       | `explorePrompt`                    | POST | `ai-search.ts`                   | Buffy |             | TODO   |
|   3 | audit           | `startAudit`                       | POST | `audit.ts`                       | Buffy |             | TODO   |
|   4 | audit           | `getAuditStatus`                   | POST | `audit.ts`                       | Buffy |             | TODO   |
|   5 | audit           | `getAuditResults`                  | POST | `audit.ts`                       | Buffy |             | TODO   |
|   6 | audit           | `getAuditHistory`                  | POST | `audit.ts`                       | Buffy |             | TODO   |
|   7 | audit           | `getCrawlProgress`                 | POST | `audit.ts`                       | Buffy |             | TODO   |
|   8 | audit           | `deleteAudit`                      | POST | `audit.ts`                       | Buffy |             | TODO   |
|   9 | audit           | `getAuditCapabilities`             | POST | `audit.ts`                       | Buffy |             | TODO   |
|  10 | audit           | `listCrawlerCredentials`           | GET  | `crawlerAccess.ts`               | Buffy |             | TODO   |
|  11 | audit           | `saveCrawlerCredential`            | POST | `crawlerAccess.ts`               | Buffy |             | TODO   |
|  12 | audit           | `deleteCrawlerCredential`          | POST | `crawlerAccess.ts`               | Buffy |             | TODO   |
|  13 | audit           | `getAuditLighthouseIssues`         | POST | `lighthouse.ts`                  | Buffy |             | TODO   |
|  14 | audit           | `exportAuditLighthouseIssues`      | POST | `lighthouse.ts`                  | Buffy |             | TODO   |
|  15 | backlinks       | `getAhrefsDomainRatings`           | POST | `ahrefs.ts`                      | Buffy |             | TODO   |
|  16 | backlinks       | `getBacklinksOverview`             | GET  | `backlinks.ts`                   | Buffy |             | TODO   |
|  17 | backlinks       | `getBacklinksRows`                 | GET  | `backlinks.ts`                   | Buffy |             | TODO   |
|  18 | backlinks       | `getBacklinksReferringDomains`     | GET  | `backlinks.ts`                   | Buffy |             | TODO   |
|  19 | backlinks       | `getBacklinksTopPages`             | GET  | `backlinks.ts`                   | Buffy |             | TODO   |
|  20 | billing         | `getBillingUsageEvents`            | POST | `billing.ts`                     | Buffy |             | TODO   |
|  21 | branding        | `getBranding`                      | GET  | `branding.ts`                    | Buffy |             | TODO   |
|  22 | branding        | `saveBranding`                     | POST | `branding.ts`                    | Buffy |             | TODO   |
|  23 | branding        | `resetBranding`                    | POST | `branding.ts`                    | Buffy |             | TODO   |
|  24 | dashboard       | `getDashboardActivation`           | POST | `dashboard.ts`                   | Buffy |             | TODO   |
|  25 | dashboard       | `getDashboardOverview`             | POST | `dashboard.ts`                   | Buffy |             | TODO   |
|  26 | dashboard       | `refreshDashboardBacklinkSnapshot` | GET  | `dashboard.ts`                   | Buffy |             | TODO   |
|  27 | dashboard       | `markDashboardStepClicked`         | POST | `dashboard.ts`                   | Buffy |             | TODO   |
|  28 | dashboard       | `dismissDashboardGa4Card`          | POST | `dashboard.ts`                   | Buffy |             | TODO   |
|  29 | dashboard       | `setDashboardStepDismissed`        | POST | `dashboard.ts`                   | Buffy |             | TODO   |
|  30 | ga4             | `getGa4Connection`                 | POST | `ga4.ts`                         | Buffy |             | TODO   |
|  31 | ga4             | `getGa4DashboardReport`            | POST | `ga4.ts`                         | Buffy |             | TODO   |
|  32 | ga4             | `listGa4Properties`                | POST | `ga4.ts`                         | Buffy |             | TODO   |
|  33 | ga4             | `setGa4Property`                   | POST | `ga4.ts`                         | Buffy |             | TODO   |
|  34 | ga4             | `disconnectGa4`                    | POST | `ga4.ts`                         | Buffy |             | TODO   |
|  35 | ga4             | `startGa4Link`                     | POST | `ga4.ts`                         | Buffy |             | TODO   |
|  36 | google          | `getGoogleAccountRemovalImpact`    | POST | `googleAccounts.ts`              | Buffy |             | TODO   |
|  37 | google          | `removeGoogleAccount`              | POST | `googleAccounts.ts`              | Buffy |             | TODO   |
|  38 | gsc             | `getGscGrantStatus`                | GET  | `gsc.ts`                         | Buffy |             | TODO   |
|  39 | gsc             | `getGscConnection`                 | POST | `gsc.ts`                         | Buffy |             | TODO   |
|  40 | gsc             | `listGscSites`                     | POST | `gsc.ts`                         | Buffy |             | TODO   |
|  41 | gsc             | `setGscSite`                       | POST | `gsc.ts`                         | Buffy |             | TODO   |
|  42 | gsc             | `disconnectGsc`                    | POST | `gsc.ts`                         | Buffy |             | TODO   |
|  43 | gsc             | `startGscLink`                     | POST | `gsc.ts`                         | Buffy |             | TODO   |
|  44 | gsc             | `getSearchPerformanceReport`       | POST | `searchPerformance.ts`           | Buffy |             | TODO   |
|  45 | gsc             | `getSearchPerformanceTable`        | POST | `searchPerformance.ts`           | Buffy |             | TODO   |
|  46 | gsc             | `exportSearchPerformanceTable`     | POST | `searchPerformance.ts`           | Buffy |             | TODO   |
|  47 | reports         | `listReportTemplates`              | POST | `reportTemplates.ts`             | Buffy |             | TODO   |
|  48 | reports         | `saveReportTemplate`               | POST | `reportTemplates.ts`             | Buffy |             | TODO   |
|  49 | reports         | `deleteReportTemplate`             | POST | `reportTemplates.ts`             | Buffy |             | TODO   |
|  50 | reports         | `listReports`                      | POST | `reports.ts`                     | Buffy |             | TODO   |
|  51 | reports         | `getReport`                        | POST | `reports.ts`                     | Buffy |             | TODO   |
|  52 | reports         | `shareReport`                      | POST | `reports.ts`                     | Buffy |             | TODO   |
|  53 | reports         | `unshareReport`                    | POST | `reports.ts`                     | Buffy |             | TODO   |
|  54 | reports         | `deleteReport`                     | POST | `reports.ts`                     | Buffy |             | TODO   |
|  55 | sam             | `listSamSessions`                  | GET  | `sam.ts`                         | Buffy |             | TODO   |
|  56 | sam             | `createSamSession`                 | POST | `sam.ts`                         | Buffy |             | TODO   |
|  57 | sam             | `archiveSamSession`                | POST | `sam.ts`                         | Buffy |             | TODO   |
|  58 | sam             | `getSamAccessSetupStatus`          | GET  | `samAccess.ts`                   | Buffy |             | TODO   |
|  59 | domain          | `getDomainOverview`                | POST | `domain.ts`                      | Codex |             | TODO   |
|  60 | domain          | `getDomainKeywordSuggestions`      | POST | `domain.ts`                      | Codex |             | TODO   |
|  61 | domain          | `getDomainKeywordsPage`            | POST | `domain.ts`                      | Codex |             | TODO   |
|  62 | domain          | `getDomainPagesPage`               | POST | `domain.ts`                      | Codex |             | TODO   |
|  63 | identity        | `getSeoApiKeyStatus`               | GET  | `config.ts`                      | Codex |             | TODO   |
|  64 | identity        | `getOnboardingAnswers`             | GET  | `onboarding.ts`                  | Codex |             | TODO   |
|  65 | identity        | `saveOnboardingAnswers`            | POST | `onboarding.ts`                  | Codex |             | TODO   |
|  66 | identity        | `dismissGscNudge`                  | POST | `onboarding.ts`                  | Codex |             | TODO   |
|  67 | identity        | `getOrganizationContext`           | GET  | `organization.ts`                | Codex |             | TODO   |
|  68 | identity        | `getTeam`                          | GET  | `organization.ts`                | Codex |             | TODO   |
|  69 | identity        | `switchOrganization`               | POST | `organization.ts`                | Codex |             | TODO   |
|  70 | identity        | `sendTeamInvitation`               | POST | `organization.ts`                | Codex |             | TODO   |
|  71 | identity        | `transferOwnership`                | POST | `organization.ts`                | Codex |             | TODO   |
|  72 | identity        | `getWorkspaceMergeStatus`          | POST | `workspace.ts`                   | Codex |             | TODO   |
|  73 | identity        | `mergeLegacyWorkspaces`            | POST | `workspace.ts`                   | Codex |             | TODO   |
|  74 | keywords        | `researchKeywords`                 | POST | `keywords.ts`                    | Codex |             | TODO   |
|  75 | keywords        | `saveKeywords`                     | POST | `keywords.ts`                    | Codex |             | TODO   |
|  76 | keywords        | `getSavedKeywords`                 | POST | `keywords.ts`                    | Codex |             | TODO   |
|  77 | keywords        | `exportSavedKeywords`              | POST | `keywords.ts`                    | Codex |             | TODO   |
|  78 | keywords        | `updateSavedKeywordTags`           | POST | `keywords.ts`                    | Codex |             | TODO   |
|  79 | keywords        | `updateSavedKeywordTag`            | POST | `keywords.ts`                    | Codex |             | TODO   |
|  80 | keywords        | `deleteSavedKeywordTag`            | POST | `keywords.ts`                    | Codex |             | TODO   |
|  81 | keywords        | `removeSavedKeywords`              | GET  | `keywords.ts`                    | Codex |             | TODO   |
|  82 | keywords        | `refreshSavedKeywordMetrics`       | POST | `keywords.ts`                    | Codex |             | TODO   |
|  83 | keywords        | `getSerpAnalysis`                  | POST | `keywords.ts`                    | Codex |             | TODO   |
|  84 | keywords        | `searchSerpLocations`              | POST | `serp-locations.ts`              | Codex |             | TODO   |
|  85 | keywords        | `prewarmSerpLocations`             | POST | `serp-locations.ts`              | Codex |             | TODO   |
|  86 | project-context | `getProjectContext`                | POST | `projectContext.ts`              | Codex |             | TODO   |
|  87 | project-context | `updateProjectContext`             | POST | `projectContext.ts`              | Codex |             | TODO   |
|  88 | projects        | `getProjects`                      | POST | `projects.ts`                    | Codex |             | TODO   |
|  89 | projects        | `createProject`                    | POST | `projects.ts`                    | Codex |             | TODO   |
|  90 | projects        | `updateProject`                    | POST | `projects.ts`                    | Codex |             | TODO   |
|  91 | projects        | `archiveProject`                   | POST | `projects.ts`                    | Codex |             | TODO   |
|  92 | projects        | `getArchivedProjects`              | POST | `projects.ts`                    | Codex |             | TODO   |
|  93 | projects        | `restoreProject`                   | POST | `projects.ts`                    | Codex |             | TODO   |
|  94 | projects        | `getProjectAccess`                 | POST | `projects.ts`                    | Codex |             | TODO   |
|  95 | rank-tracking   | `getRankTrackingConfigs`           | POST | `rank-tracking.ts`               | Codex |             | TODO   |
|  96 | rank-tracking   | `getRankTrackingConfigSummaries`   | POST | `rank-tracking.ts`               | Codex |             | TODO   |
|  97 | rank-tracking   | `createRankTrackingConfig`         | POST | `rank-tracking.ts`               | Codex |             | TODO   |
|  98 | rank-tracking   | `updateRankTrackingConfig`         | POST | `rank-tracking.ts`               | Codex |             | TODO   |
|  99 | rank-tracking   | `triggerRankCheck`                 | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 100 | rank-tracking   | `getLatestRankResults`             | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 101 | rank-tracking   | `getLatestRankRun`                 | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 102 | rank-tracking   | `estimateRankCheckCost`            | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 103 | rank-tracking   | `addTrackingKeywords`              | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 104 | rank-tracking   | `removeTrackingKeywords`           | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 105 | rank-tracking   | `refreshTrackingKeywordMetrics`    | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 106 | rank-tracking   | `getRankKeywordHistory`            | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 107 | rank-tracking   | `getRankConfigTrend`               | POST | `rank-tracking.ts`               | Codex |             | TODO   |
| 108 | rank-tracking   | `getRankPositionMatrix`            | POST | `rank-tracking.ts`               | Codex |             | TODO   |

## 2. Server-side routes to port (not UI)

These live in `src/routes` but run on the server. They are NOT counted in `STATUS.md` file totals, so they must be ported too.

| Route file (`src/routes/`)               | Purpose (verify in code)                                           | Owner | Go endpoint | Status |
| ---------------------------------------- | ------------------------------------------------------------------ | ----- | ----------- | ------ |
| `[.well-known]/openai-apps-challenge.ts` | OpenAI apps verification                                           | Codex |             | TODO   |
| `api/auth/$.ts`                          | better-auth sign-in/up, sessions (Go only reads the session today) | Codex |             | TODO   |
| `api/autumn/$.ts`                        | billing provider endpoint                                          | Buffy |             | TODO   |
| `api/ga4/oauth/callback.ts`              | GA4 OAuth callback                                                 | Buffy |             | TODO   |
| `api/gsc/oauth/callback.ts`              | Search Console OAuth callback                                      | Buffy |             | TODO   |
| `api/health.ts`                          | health check (Go already has /healthz, /readyz)                    | Codex |             | TODO   |
| `r/$reportId.ts`                         | public report page                                                 | Buffy |             | TODO   |
| `s/$token/index.ts`                      | shared report view                                                 | Buffy |             | TODO   |
| `s/$token/og[.]png.ts`                   | shared report social image                                         | Buffy |             | TODO   |
| `s/$token/raw.ts`                        | shared report raw data                                             | Buffy |             | TODO   |

## 3. MCP tools (Wave 2, Codex)

|   # | Tool                                         | TS file (`src/server/mcp/tools/`)   | Go  | Status |
| --: | -------------------------------------------- | ----------------------------------- | --- | ------ |
|   1 | `add_rank_tracking_keywords`                 | `add-rank-tracking-keywords.ts`     |     | TODO   |
|   2 | `create_project`                             | `create-project.ts`                 |     | TODO   |
|   3 | `create_rank_tracker`                        | `create-rank-tracker.ts`            |     | TODO   |
|   4 | `get_ranked_keywords`                        | `dataforseo-research-tools.ts`      |     | TODO   |
|   5 | `search_local_businesses`                    | `dataforseo-research-tools.ts`      |     | TODO   |
|   6 | `get_local_serp_results`                     | `dataforseo-research-tools.ts`      |     | TODO   |
|   7 | `get_google_business_questions`              | `dataforseo-research-tools.ts`      |     | TODO   |
|   8 | `find_serp_competitors`                      | `dataforseo-research-tools.ts`      |     | TODO   |
|   9 | `get_keyword_metrics`                        | `dataforseo-research-tools.ts`      |     | TODO   |
|  10 | `estimate_rank_tracker_cost`                 | `estimate-rank-tracker-cost.ts`     |     | TODO   |
|  11 | `get_backlinks_overview`                     | `get-backlinks-overview.ts`         |     | TODO   |
|  12 | `get_backlinks_profile`                      | `get-backlinks-profile.ts`          |     | TODO   |
|  13 | `get_domain_keyword_suggestions`             | `get-domain-keyword-suggestions.ts` |     | TODO   |
|  14 | `get_domain_overview`                        | `get-domain-overview.ts`            |     | TODO   |
|  15 | `get_rank_tracker`                           | `get-rank-tracker.ts`               |     | TODO   |
|  16 | `get_serp_results`                           | `get-serp-results.ts`               |     | TODO   |
|  17 | `get_google_analytics_organic_landing_pages` | `google-analytics-tools.ts`         |     | TODO   |
|  18 | `get_google_analytics_page_performance`      | `google-analytics-tools.ts`         |     | TODO   |
|  19 | `get_google_analytics_key_events`            | `google-analytics-tools.ts`         |     | TODO   |
|  20 | `get_search_opportunities`                   | `google-analytics-tools.ts`         |     | TODO   |
|  21 | `get_google_analytics_organic_overview`      | `google-analytics-tools.ts`         |     | TODO   |
|  22 | `get_google_analytics_traffic_acquisition`   | `google-analytics-tools.ts`         |     | TODO   |
|  23 | `get_google_analytics_ecommerce_performance` | `google-analytics-tools.ts`         |     | TODO   |
|  24 | `get_google_analytics_site_search`           | `google-analytics-tools.ts`         |     | TODO   |
|  25 | `get_google_analytics_audience_breakdown`    | `google-analytics-tools.ts`         |     | TODO   |
|  26 | `get_google_analytics_measurement_health`    | `google-analytics-tools.ts`         |     | TODO   |
|  27 | `list_projects`                              | `list-projects.ts`                  |     | TODO   |
|  28 | `list_saved_keywords`                        | `list-saved-keywords.ts`            |     | TODO   |
|  29 | `get_business_profile`                       | `local-seo-tools.ts`                |     | TODO   |
|  30 | `get_business_reviews`                       | `local-seo-tools.ts`                |     | TODO   |
|  31 | `get_business_updates`                       | `local-seo-tools.ts`                |     | TODO   |
|  32 | `list_business_categories`                   | `local-seo-tools.ts`                |     | TODO   |
|  33 | `get_local_rank_grid`                        | `local-seo-tools.ts`                |     | TODO   |
|  34 | `get_project_context`                        | `project-context.ts`                |     | TODO   |
|  35 | `remove_rank_tracking_keywords`              | `remove-rank-tracking-keywords.ts`  |     | TODO   |
|  36 | `remove_saved_keywords`                      | `remove-saved-keywords.ts`          |     | TODO   |
|  37 | `set_report_sharing`                         | `report-sharing-tools.ts`           |     | TODO   |
|  38 | `list_report_templates`                      | `report-template-tools.ts`          |     | TODO   |
|  39 | `save_report_template`                       | `report-template-tools.ts`          |     | TODO   |
|  40 | `delete_report_template`                     | `report-template-tools.ts`          |     | TODO   |
|  41 | `save_report`                                | `report-tools.ts`                   |     | TODO   |
|  42 | `list_reports`                               | `report-tools.ts`                   |     | TODO   |
|  43 | `get_report`                                 | `report-tools.ts`                   |     | TODO   |
|  44 | `delete_report`                              | `report-tools.ts`                   |     | TODO   |
|  45 | `research_keywords`                          | `research-keywords.ts`              |     | TODO   |
|  46 | `run_rank_tracker`                           | `run-rank-tracker.ts`               |     | TODO   |
|  47 | `save_keywords`                              | `save-keywords.ts`                  |     | TODO   |
|  48 | `get_search_console_performance`             | `search-console-tools.ts`           |     | TODO   |
|  49 | `inspect_urls`                               | `search-console-tools.ts`           |     | TODO   |
|  50 | `search_serp_locations`                      | `search-serp-locations.ts`          |     | TODO   |
|  51 | `list_site_audits`                           | `site-audit-cleanup-tools.ts`       |     | TODO   |
|  52 | `delete_site_audit`                          | `site-audit-cleanup-tools.ts`       |     | TODO   |
|  53 | `run_site_audit`                             | `site-audit-tools.ts`               |     | TODO   |
|  54 | `get_audit_status`                           | `site-audit-tools.ts`               |     | TODO   |
|  55 | `get_audit_issues`                           | `site-audit-tools.ts`               |     | TODO   |
|  56 | `get_audit_pages`                            | `site-audit-tools.ts`               |     | TODO   |
|  57 | `whoami`                                     | `whoami.ts`                         |     | TODO   |
|  58 | `ping`                                       | `server.ts`                         |     | TODO   |

## 4. UI pages (stay React; must keep working against the Go API)

Smoke-test each page after its backend moves. Do not delete.

| Page (`src/routes/`)                            | Backend it depends on | Checked against Go |
| ----------------------------------------------- | --------------------- | ------------------ |
| `__root.tsx`                                    |                       |                    |
| `_app/ai.tsx`                                   |                       |                    |
| `_app/billing.tsx`                              |                       |                    |
| `_app/billing_.fix-payment.tsx`                 |                       |                    |
| `_app/billing_.plan.tsx`                        |                       |                    |
| `_app/help/dataforseo-api-key.tsx`              |                       |                    |
| `_app/help/openrouter-api-key.tsx`              |                       |                    |
| `_app/index.tsx`                                |                       |                    |
| `_app/p/$projectId/analytics.tsx`               |                       |                    |
| `_app/p/$projectId/audit/index.tsx`             |                       |                    |
| `_app/p/$projectId/audit/issues/$resultId.tsx`  |                       |                    |
| `_app/p/$projectId/backlinks.tsx`               |                       |                    |
| `_app/p/$projectId/brand-lookup.tsx`            |                       |                    |
| `_app/p/$projectId/context.tsx`                 |                       |                    |
| `_app/p/$projectId/domain.tsx`                  |                       |                    |
| `_app/p/$projectId/index.tsx`                   |                       |                    |
| `_app/p/$projectId/keywords.tsx`                |                       |                    |
| `_app/p/$projectId/prompt-explorer.tsx`         |                       |                    |
| `_app/p/$projectId/rank-tracking.tsx`           |                       |                    |
| `_app/p/$projectId/rank-tracking/$configId.tsx` |                       |                    |
| `_app/p/$projectId/rank-tracking/index.tsx`     |                       |                    |
| `_app/p/$projectId/reports/$reportId.tsx`       |                       |                    |
| `_app/p/$projectId/reports/index.tsx`           |                       |                    |
| `_app/p/$projectId/reports/templates.tsx`       |                       |                    |
| `_app/p/$projectId/route.tsx`                   |                       |                    |
| `_app/p/$projectId/sam.tsx`                     |                       |                    |
| `_app/p/$projectId/saved.tsx`                   |                       |                    |
| `_app/p/$projectId/search-performance.tsx`      |                       |                    |
| `_app/p/$projectId/settings.tsx`                |                       |                    |
| `_app/p/$projectId/settings/context.tsx`        |                       |                    |
| `_app/p/$projectId/settings/index.tsx`          |                       |                    |
| `_app/p/$projectId/settings/integrations.tsx`   |                       |                    |
| `_app/projects.tsx`                             |                       |                    |
| `_app/route.tsx`                                |                       |                    |
| `_app/settings.tsx`                             |                       |                    |
| `_app/settings/branding.tsx`                    |                       |                    |
| `_app/settings/index.tsx`                       |                       |                    |
| `_app/settings/organization.tsx`                |                       |                    |
| `_app/support.tsx`                              |                       |                    |
| `_auth.sign-in.tsx`                             |                       |                    |
| `_auth.sign-up.tsx`                             |                       |                    |
| `_auth.tsx`                                     |                       |                    |
| `_authenticated.oauth-consent.tsx`              |                       |                    |
| `_authenticated.onboarding.index.tsx`           |                       |                    |
| `_authenticated.subscribe.tsx`                  |                       |                    |
| `_authenticated.tsx`                            |                       |                    |
| `_authenticated.yc.tsx`                         |                       |                    |
| `accept-invitation.$id.tsx`                     |                       |                    |
| `auth-error.tsx`                                |                       |                    |
| `forgot-password.tsx`                           |                       |                    |
| `reset-password.tsx`                            |                       |                    |
| `verify-email.tsx`                              |                       |                    |

## 5. Frontend feature folders (stay React)

`ai-mcp`, `ai-search`, `analytics`, `audit`, `auth`, `backlinks`, `billing`, `crawler-access`, `dashboard`, `domain`, `gsc`, `help`, `integrations`, `keywords`, `lighthouse`, `onboarding`, `projects`, `rank-tracking`, `reports`, `sam`, `saved-keywords`, `search-performance`, `search-tabs`, `settings`, `team`
