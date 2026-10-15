# Uplumo: owner decisions and what has been done

Owner: Toufiq. This file records the owner's decisions and what still waits
on the owner. Feature status lives in `ROADMAP.md`, history in git.
**No secret values here, only variable names.** Agents: keep this file and
update it when the owner decides something or a Railway variable changes.

Last updated: 15 Oct 2026

---

## 1. Decisions taken by the owner

| Date | Decision | Why |
|---|---|---|
| 14 Oct | WordPress publishing is on the **Basic, Pro and Agency** plans. Free gets a link to the plans. | It is a paid feature with no provider cost. |
| 14 Oct | Only an **owner or admin** can connect a website. Any member can publish. | The site password is sensitive. |
| 14 Oct | Onboarding has a **6th step**, "connect website", and it can be skipped. | Customers can connect early without being blocked. |
| 14 Oct | We **never** ask for cPanel or hPanel logins. We use only the WordPress application password. | Full server access would be too risky for us. |
| 14 Oct | Shopify, Wix, Webflow, SFTP and a hosted blog come **later**. WordPress is first. | Most target sites run on WordPress. |
| 14 Oct | Content editor: a **real scoring engine** with SERP briefs and AI help, built by Codex in 3 PRs. | The old score gave 26 words of spam 65/100. |
| 14 Oct | A brief = Google top 10 + "People also ask" only, **no keyword-volume call**. Cost about $0.0065, **9 credits** per brief. | Volume and difficulty do not change the score, and they were 65% of the brief's cost. |
| 14 Oct | Volume and difficulty come from a **"See keyword data →"** link to the Keyword Research page. | Data stays available, and is charged only when the user asks. |
| 14 Oct | The brief cache is **shared by all workspaces** for 30 days. Only a cache miss is charged. The same keyword at the same time makes one provider call. | Lower DataForSEO bill. |
| 14 Oct | **Free plan:** no brief and no AI, only the local checks. | Cost control. |
| 14 Oct | Scoring weights and caps follow the Codex proposal: stuffing → max 40 and "Publish now" blocked; copied text → max 30; under 100 words → max 30. | Writers cannot game the score with spam. |
| 14 Oct | AI model `openai/gpt-4.1-mini`, 10 actions, 11–15 credits each. Credits are reserved before a call and fully released if the provider fails. | Fixed cost; the customer never pays for our failure. |
| 14 Oct | The keyword-stuffing block applies on the **server** too, not only on screen. "Send as draft" still works. | An API or agent cannot publish spam to a customer's site. |
| 15 Oct | The PageSpeed API key has **API restrictions only** (PageSpeed and CrUX) and application restriction **None**. | The server sends no referrer, so a "Websites" restriction would block it. Railway has no fixed IP. |
| 15 Oct | `GOOGLE_TOKEN_ENCRYPTION_KEY` and `AUDIT_BROWSER_RENDERING` are **not set for now**. | The first would break existing Google connections; the second needs Chrome on the server and a lot of memory. |
| 15 Oct | A search brief reads 8 competitor pages (worst case $0.0072, about 9.2 credits) instead of 10, with one content-parsing call per page. | DataForSEO's live parsing endpoint takes one page per call. 8 pages keep the brief near the approved 9 credits. |
| 15 Oct | The AI writer always uses `openai/gpt-4.1-mini`, whatever `OPENROUTER_MODEL` says. Free OpenRouter models are not used. | The per-action credit ceilings are priced for that model. Free models have tight rate limits and may train on customer content. |
| 15 Oct | "Track this article" opens Rank Tracking and Prompt Explorer after publishing; no separate scheduled tracking yet. | Reuses the existing tools and their credit checks instead of a second scheduler. |
| 15 Oct | Migration `00023` is the brief cache. The next migration (Buffy's author tables) must be `00024`. | Two PRs used `00023`; goose refuses a gap or a duplicate. |

## 2. Waiting for the owner

- [ ] **DataForSEO:** verify the account and top up (at least $50). Until then these return 40104 "verify your account": keyword research, domain, backlinks, brand lookup, prompt explorer, rank tracking and local SEO. No money is charged meanwhile.
- [ ] **Google OAuth data-access verification:** add the scope justification, upload the demo video (YouTube, unlisted), then submit in the Verification Center. Branding is already verified.
- [ ] **GA4 connect:** use "Advanced → Go to uplumo" until Google verifies the app. The user cap is 100, and 1 is used.
- [ ] **WordPress (revmerito.com):** add the 2 `.htaccess` lines on Hostinger (the host drops the Authorization header), then connect with a real application password (24 characters).
- [ ] **Railway GitHub auto-deploy** is broken: Railway shows "Could not load branches". Reconnect GitHub in Railway Account Settings → Connections. Until then, deploy with `railway up` (see `docs/DEPLOY.md`).
- [ ] Mark the 2 GitGuardian incidents from PR #122 as false positives.
- [ ] Codex PR 2 (SERP briefs and credits) and PR 3 (AI actions) still have to be built.

## 3. Keys and settings in Railway (names only)

| Variable | Status | What it is for |
|---|---|---|
| `DATABASE_URL` | set | Supabase Postgres |
| `REDIS_URL` | set | Railway Redis |
| `BETTER_AUTH_SECRET`, `BETTER_AUTH_URL`, `PUBLIC_URL` | set | Login and links |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | set | Google login, Search Console, GA4 |
| `RAZORPAY_KEY_ID`, `RAZORPAY_KEY_SECRET`, `RAZORPAY_WEBHOOK_SECRET` | set (**test mode**) | Billing |
| `RAZORPAY_PLAN_ID_BASIC`, `_PRO`, `_AGENCY` | set (test) | Monthly plans |
| `LOOPS_API_KEY` + 3 `LOOPS_TRANSACTIONAL_*_ID` | set | Email |
| `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, `SUPABASE_CONTENT_IMAGE_BUCKET` | set (14 Oct) | Content editor images (`content-images` bucket) |
| `DATAFORSEO_API_KEY` | **set 14 Oct** | SEO data; the account is not verified yet |
| `OPENROUTER_API_KEY`, `OPENROUTER_MODEL` | the variable exists; its value has not been checked | AI writing and assistant |
| `PAGESPEED_API_KEY` | **set 15 Oct** | Lighthouse in Site Audit (free, 200 checks per workspace per day) |
| `CRUX_API_KEY` | **set 15 Oct** (same key) | Real-visitor Core Web Vitals |
| `SENTRY_DSN`, `STAFF_EMAILS`, `PROVIDER_DAILY_LIMIT_USD`, `PORT` | set | Errors, staff access, emergency spending limit |
| `TURNSTILE_SECRET_KEY` | not set | Bot check on sign-up |
| `GOOGLE_TOKEN_ENCRYPTION_KEY` | not set (decision above) | — |

Railway service settings: healthcheck `/readyz`, region Singapore, 1 replica.

Google Cloud project `agentproject-473109`:

- OAuth app is "In production"; branding is verified.
- Scopes: `openid`, email, profile, `webmasters.readonly` and `analytics.readonly`. The last one needs verification.
- APIs enabled: PageSpeed Insights and Chrome UX Report.

## 4. What was built and shipped (PRs)

| PR | What | Live |
|---|---|---|
| #120 (Codex) | New content editor, PNG/JPEG image upload to Supabase (WebP), Markdown preview | yes |
| #121 | Billing page waits for the plan to change after a payment | yes |
| #122 | WordPress connect plus "Send as draft / Publish now" | yes |
| #123 | The dashboard toast "Something went wrong" (backlinks panic without DataForSEO) | yes |
| #124 | WordPress connect: a clear message when a firewall blocks us | yes |
| #125 (Codex) | Scoring engine plus new editor UI: stuffing and copy caps, SEO/AEO checks, brief, outline | yes |
| #126 | Server refuses "Publish now" on a stuffed draft; `DEPLOY.md` corrected | yes |
| #127 | Live-test fixes: preview formatting, "Your drafts" shown twice, provider outage message, audit polling halved, 0-word read time, Hostinger dropping the login header | yes (12:46 UTC, 14 Oct) |

## 5. Live test results (14 Oct, owner's Chrome)

- All 15 pages load with no console errors. Every API call answers 200, and no paid call is made when a page opens.
- Content editor: a 265-word article scored SEO 82 and AEO 85. Typing makes 0 API calls. Save, reload and reopen work. The image upload reaches Supabase and shows in the preview.
- Site audit on revmerito.com, 10 pages: 25 issues (3 warnings, 22 info).
- Security: another workspace's project answers 404.
- Blocked by DataForSEO verification: keyword research and prompt explorer.
- Blocked by Hostinger: WordPress connect (the `.htaccess` fix is pending).

## 6. Cost notes

- A content brief costs about $0.0065 (9 credits). Pro (10,000 credits) ≈ 1,111 briefs a month. The daily limit per workspace is a third of its monthly credits.
- An AI action costs at most 11–15 credits.
- PageSpeed and CrUX are free (Google quota).
- 1 credit = $1/1,280 of provider cost.
