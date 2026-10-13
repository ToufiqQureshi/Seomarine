# Uplumo dashboard functional QA

**Test date:** 2026-10-13  
**Target:** `https://uplumo.com` production  
**Client:** Chrome 154 on Windows; desktop and 375 × 812 mobile viewport  
**Account state:** Signed-in account, Free plan, zero credits; one Default project

## Result

The dashboard shell and the primary navigation routes render. Search Console data requests return HTTP 200, but the connected property currently has no report rows. Several important product paths cannot complete in this account: payment configuration is disabled, GA4 needs Google consent, analytics has no visits, and paid SEO/AI operations have no credits. One confirmed API payload bug breaks Saved Keywords and the shared keyword API request helper; a code fix is prepared separately below.

## Pages and features visited

All sidebar routes were opened and their page headings/content inspected: Dashboard, Analytics, Search Performance, Brand Lookup, Prompt Explorer, Content Editor, Rank Tracking, Local SEO, Site Audit, Saved Keywords, Keyword Research, Domain Overview, Backlinks, Reports, Context, and Agent setup. Account-level Projects, Billing, Help, Settings, Organization, and Branding screens, plus project General and Integrations settings, were also inspected.

Read-only interactions exercised included Search Console Queries/Pages/Striking-distance tabs, filter-panel and export-menu opening, sort controls, date-range selection, analytics 7/30/90-day controls, tracking-code reveal and “check again”, report-template navigation and new-template dialog open/cancel, and the mobile navigation drawer. Analytics range controls updated the URL and fetched matching summary dates. The mobile drawer opened with all navigation links.

## Findings

### High: keyword API sends a path parameter again in the JSON body

Live Saved Keywords sends `projectId` in the POST body even though the project ID is already part of the URL. The Go handler rejects unknown JSON fields and responds `400 invalid_request` (“The request is not valid JSON.”). The shared frontend helper is used by keyword research, SERP, and saved-keyword endpoints, so those operations are at risk of the same failure.

**Fix prepared:** `postKeyword` now removes `projectId` from the body while retaining it in the encoded route. The API test assertions cover that request shape. Focused verification passed: 14 tests and TypeScript `tsc --noEmit`.

### High: paid plans and checkout are unavailable in production

The billing credits API returned `paymentsConfigured: false`, `plan: free`, and a zero balance. Billing shows an upgrade offer, but `/subscribe?upgrade=true` renders “Payments unavailable”; the user cannot complete checkout. The Billing screen also displays an older Pro offer at ₹1,499/month with all-feature access and $10 usage credits, while the requested pricing is Basic ₹899, Pro ₹1,899, and Agency ₹4,999. Align the product catalog, feature limits, checkout configuration, and billing copy before selling these plans.

### High: GA4 reporting could not be verified

Google Analytics is shown as Not connected. The Connect action reaches Google’s OAuth consent flow and requests the `analytics.readonly` scope. No consent was granted during QA, so no GA4 property was linked and its report data path remains unverified.

### Medium: Search Console is connected but currently returns no data

The project Integrations page reports a connected Search Console property. Search Performance summary and table requests both return HTTP 200, but all totals are zero and the Queries/Pages views show “No Search Console data yet”. This may be a genuinely empty/new property; compare against Search Console directly before treating zero as correct. Query/page tabs and the date-range control function, but there are no rows with which to verify sorting, filtering, pagination, or export contents.

### Medium: first-party analytics has no collected visits

The Analytics page returns HTTP 200 for 7-, 30-, and 90-day ranges, all with zero visits. It displays a valid tracking snippet and an install-check action. The project General settings show no domain configured, so tracking installation and end-to-end event collection could not be confirmed against a site. Verify the project domain and install the snippet on the intended website.

### Low: form fields lack accessible identifiers

Content Editor logged two form controls without an `id` or `name`; Local SEO logged seven. Add stable labels/identifiers and confirm labels are programmatically associated with their inputs.

### Low: browser console warning and Lighthouse console audit

Mobile Lighthouse on Search Performance scored Accessibility 100, Best Practices 96, SEO 100, and Agentic Browsing 97. Its console-error audit failed because Chrome reported one resource blocked by the client; this is consistent with a browser extension/privacy filter and is not enough to attribute to the application. The browser also warns that `apple-mobile-web-app-capable` is deprecated; add `mobile-web-app-capable` where applicable.

## Not run on production

Keyword research, SERP lookup, Brand Lookup, Prompt Explorer, AI generation, rank refresh, Local SEO lookup, and Site Audit crawl were not submitted. These can call paid providers or create usage. The account currently has zero credits and payment configuration is disabled. Saving/deleting keywords, changing project settings, disconnecting Search Console, changing branding, sending team invitations, creating API keys, and altering account preferences were also not performed because they change persistent account data. GA4 consent was intentionally left for the account owner.

Therefore this is a live route and safe-interaction audit, not a successful end-to-end validation of paid data-provider jobs or external OAuth completion. Those flows need a funded test account, a configured website, and authorized GA4 consent to validate fully.

## Earlier related audit

The public-site, legal-page, and initial pricing review is documented in [`2026-10-13-uplumo-live-site-pricing-legal-audit.md`](./2026-10-13-uplumo-live-site-pricing-legal-audit.md).
