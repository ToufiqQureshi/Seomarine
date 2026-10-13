# Uplumo live-site audit, pricing proposal, and legal-content review

**Date:** 13 October 2026  
**Site:** https://uplumo.com/  
**Method:** Chrome desktop, 390 px mobile viewport, signed-out and an existing signed-in workspace. This is a black-box review. Source code, billing settlement, real email delivery, provider invoices, and backend entitlement rules were not inspected.

## Main findings

1. **P0: paid conversion is blocked.** The signed-in Billing page shows “Something went wrong on our side. Try again.” The upgrade page shows “Billing unavailable.” Retry did not recover either page. Chrome Network recorded HTTP 503 from GET /api/v1/billing/status and GET /api/v1/billing/credits. Fix and retest this before launching new plans.
2. **The live offer is still Free ₹0 and Pro ₹1,499/month.** Home, pricing, FAQ, terms, refunds, and delivery copy all use this offer. Basic ₹899, Pro ₹1,899, and Agency ₹4,999 are the requested future offer, not currently live.
3. **Cost boundaries need server verification.** In the signed-in workspace, the New project form opens. Local SEO's provider-backed buttons enable when required fields are entered; the UI says Maps grid requests may be billable. I did not submit a new project or paid-provider query. This is a risk to verify, not a proven backend bypass.
4. **Upcoming features need accurate labels.** The site describes some capabilities as included today. If a capability is still being built, label it “Coming soon” and do not count it as a delivered paid benefit.

## Tested flows

| Area | Result |
|---|---|
| Public home/pricing | Main destinations inspected; five FAQ disclosures on each page opened and showed answers. Home “Start free” reached sign-up. |
| Sign-up | Email form opened; empty and malformed email, short password, and mismatched passwords produced visible inline errors. Back to signup worked. |
| Sign-in/reset | Email sign-in form opened; empty submission showed errors. Forgot-password page opened; empty reset submission showed email validation. |
| Public pages | About, Contact, Terms, Privacy, Refunds, and Delivery loaded. |
| Mobile pricing | At 390 px width, no horizontal overflow; login/start links remained visible. |
| Signed-in navigation | Dashboard, Analytics, Search Performance, Brand Lookup, Prompt Explorer, Content Editor, Rank Tracking, Local SEO, Site Audit, Saved Keywords, Keyword Research, Domain Overview, Backlinks, Reports, and Context opened. |
| AI paid gate | Brand Lookup and Prompt Explorer eventually displayed a paid-plan upgrade screen in the current workspace. |
| Billing | Billing and upgrade screens failed with repeated 503 responses. |
| Project/Local SEO forms | New project dialog opened without submitting it. Local SEO controls enabled after test fields were filled; no provider request was run. |

**Coverage limit:** I did not verify Google OAuth, account/email creation, real reset email, actual payment/refund/cancellation, paid AI answers, paid SEO and Maps data, audit jobs, tracker installation, exports with data, invitations, or account deletion. Those need a safe test account, a payment sandbox, and a controlled provider budget. This review does not establish that every button works. Brief loading/blank states on some app routes resolved after waiting and are not recorded as defects.

## Defects and risks

### P0 — Billing API 503

- **Reproduce:** In a signed-in browser, open https://uplumo.com/billing/plan, wait for loading, then press Try again. Open https://uplumo.com/subscribe?upgrade=true and press Try again.
- **Expected:** Current plan and a usable upgrade or checkout path.
- **Actual:** Both pages show persistent error states. GET /api/v1/billing/status and GET /api/v1/billing/credits returned 503 on repeated requests.
- **Fix:** Trace the endpoints through application and billing dependency logs. Restore successful responses; then test checkout, payment webhook activation, renewal, failed payment, cancellation, downgrade, and retries. Add an automated billing smoke test and alert.

### P1 — Offer mismatch at three-plan launch

The [home page](https://uplumo.com/), [pricing page](https://uplumo.com/pricing), [terms](https://uplumo.com/terms), [refunds](https://uplumo.com/refunds), and [delivery](https://uplumo.com/shipping) describe the old two-tier offer. Change these, the app's billing UI, checkout plan IDs, invoice descriptions, emails, and entitlement settings in one coordinated release. Use a versioned plan catalogue for prices and machine-enforced limits.

### P1 — Potential unbounded provider cost

The current workspace can open a New project form. The Local SEO screen has potentially billable Maps actions that become enabled after valid fields are supplied. Site Audit offers up to 10,000 pages in its input. This does not prove the server permits these operations for every plan. Test the API boundary directly with free/Basic accounts, limits reached, concurrent requests, retries, and downgrade states. Hiding or disabling a UI button is insufficient protection.

## Recommended plans — proposal, not live

Charge per **workspace per month**. An active site means a verified root domain/property. Prevent repeated domain replacement from bypassing the allowance. These numbers are initial product limits; validate provider invoices and margins before publishing them.

| Capability | Basic ₹899 | Pro ₹1,899 | Agency ₹4,999 |
|---|---:|---:|---:|
| Active sites | **1** | **3** | **10** |
| Team seats | 1 | 2 | 5 |
| First-party website analytics | Included | Included | Included |
| Connected Google Search Console performance | Included | Included | Included |
| Technical site audit | 1 run/week, 100 URLs | Weekly, 500 URLs/site | Weekly, 1,000 URLs/site |
| Search/analytics history | 90 days | 12 months | 24 months |
| Paid rank checks | None | 50 tracked keywords/week/workspace | 250 tracked keywords/week/workspace |
| Paid keyword/domain/backlink lookups | None | Small monthly shared allowance based on actual unit cost | Larger shared allowance based on actual unit cost |
| AI visibility, Brand Lookup, Prompt Explorer | None | 60 prompt-provider checks/month/workspace | 300 checks/month/workspace |
| AI writing/assistant actions | None | 20/month/workspace | 100/month/workspace |
| Reports | Standard report | Scheduled report | White-label client reports |
| Paid Local SEO/Maps requests | None | Add only after measured unit economics | Explicit capped pool after measured unit economics |

**Basic must still be useful:** traffic, Search Console queries/pages, and a limited but actionable site audit let a single-site owner do real SEO work without third-party SEO-data or LLM spend. Search Console still has per-site/user/project API quotas, so schedule/cache reads and monitor them ([official limits](https://developers.google.com/webmaster-tools/limits)).

**AI unit definition:** one prompt against one provider equals one check. Three prompts against two providers consume six checks. Show the remaining allowance and renewal date in the app. AI costs vary by model, tokens, and optional tools/grounding; do not promise unlimited use on a fixed-price plan ([Gemini billing](https://ai.google.dev/gemini-api/docs/billing/), [Gemini pricing](https://ai.google.dev/gemini-api/docs/pricing), [OpenAI API pricing](https://developers.openai.com/api/docs/pricing)). A provider API answer can differ from the consumer chat product; marketing should describe exactly what is measured.

**Engineering controls:** Authorize every costly operation on the server before invoking a provider. Atomically reserve and record usage per workspace/site/month, provider, operation, request ID, unit count, and actual cost. Make provider retries and payment webhooks idempotent. Add hard spend caps, queues/rate limits, and alerts. On downgrade, preserve data for the promised period, make excess sites read-only, and let the customer choose active sites. Determine final allowances from net revenue after tax, payment fees, infrastructure, and vendor bills.

## Legal-content review

This is a risk review, **not a legal certification**. Have an India-qualified lawyer approve the final wording, tax treatment, consumer rights, and privacy practices. The highest-risk issue is a published promise that differs from the actual service.

1. **Cancellation promise (high):** [Pricing](https://uplumo.com/pricing), [terms](https://uplumo.com/terms), and [refunds](https://uplumo.com/refunds) promise one-click cancellation. Billing currently fails. Restore and test the control. Until then, publish an effective alternate cancellation path prominently. A policy sentence does not replace a working control.
2. **Price, renewal, tax, and limits (high):** Refunds still states ₹1,499 Pro. Delivery only mentions Free/Pro. Terms says the shown price is the “full price” but also says tax may be added before payment. For the new offer, show recurring amount, billing interval, whether tax is included, renewal/cancellation terms, each site's and usage limits, and effective cancellation date before payment. Have tax counsel approve exact wording.
3. **Upcoming capabilities (high):** [Terms](https://uplumo.com/terms) and marketing describe keyword research, backlinks, AI visibility, an AI assistant, and named AI surfaces as service capabilities. Mark unreleased or unverified flows “Coming soon” on home, pricing, and plan comparison; omit them from included paid benefits until production verification. India's consumer authority publishes [misleading-advertisement guidance](https://consumeraffairs.nic.in/latestnews/guidelines-prevention-misleading-advertisements-and-endorsements-misleading).
4. **Privacy assertions (high):** [Privacy](https://uplumo.com/privacy) asserts no tracker cookies/IP storage, daily-rotating hashes, encrypted Google tokens, deletion within 30 days, and no AI-training use. Verify app code, network requests, logs, backups, deletion jobs, vendor settings and contracts before retaining these absolute statements. Replace “nobody can be followed” with a precise tested explanation of identifiers/retention. Use the [DPDP Act](https://www.meity.gov.in/static/uploads/2024/02/Digital-Personal-Data-Protection-Act-2023.pdf) and the [notified DPDP Rules](https://www.meity.gov.in/static/uploads/2025/11/53450e6e5dc0bfa85ebd78686cadad39.pdf) as the compliance baseline, noting staged commencement.
5. **Google-connected data and AI (high):** The privacy page says Search Console/Analytics figures may be sent to an AI model when a user asks. Verify minimal scopes, clear in-context disclosure/consent, provider no-training settings, and working disconnect/export/delete paths. Google requires accurate disclosure and imposes Limited Use obligations ([Google API User Data Policy](https://developers.google.com/terms/api-services-user-data-policy), [Google API Terms](https://developers.google.com/terms)).
6. **Activation inconsistency (medium):** Terms says an account works as soon as signup succeeds; delivery says after email confirmation. Pick the real rule. Paid activation “within minutes” and the 30-minute fallback also need a tested payment-webhook/recovery path.
7. **Refunds and dispute language (medium):** Review the seven-day refund-request window and no partial-month refund statement for statutory exceptions and failed delivery. Review the Mumbai-jurisdiction and liability-cap clauses so they do not imply mandatory consumer rights are waived. Counsel should assess the relevant [consumer-protection rules](https://consumeraffairs.nic.in/acts-and-rules/consumer-protection/consumer-protection) for Uplumo's customer mix.
8. **Business/grievance contact (medium):** Contact/privacy pages give a name and email but only “Mumbai, Maharashtra” as the postal location. Confirm required business identity, contact, and grievance details with counsel; publish a clear request and escalation path.

**Safer launch copy draft:** “Basic ₹899/month: one site, analytics, connected Search Console, and weekly site audits. Pro ₹1,899/month: up to three sites plus capped rank tracking and AI checks. Agency ₹4,999/month: up to ten sites, larger shared allowances, and white-label reports. See exact monthly limits and taxes before paying. Features marked Coming soon are available only after release.” Update this when the actual entitlements are final.

## Release sequence

1. Repair billing 503 responses; verify the complete subscription lifecycle with sandbox payments and a controlled production account.
2. Measure vendor unit costs, finalize allowances, implement atomic server-side entitlements and cost telemetry.
3. Ship the three-tier pricing, backend limits, checkout, invoice/email copy, and public/legal copy together.
4. Verify privacy claims against implementation and subprocessors, then obtain legal approval.
5. Test Basic, Pro, Agency, over-limit, downgrade, failed-payment, and mobile scenarios. Publish only capabilities that pass the end-to-end test; label the rest Coming soon.
