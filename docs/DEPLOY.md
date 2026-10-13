# Deploying Uplumo

Uplumo runs on Railway as one service built from the root `Dockerfile`, plus a
Postgres and a Redis database. The server applies database migrations itself
every time it starts.

Secrets go into Railway variables only. Never put them in the repo or in chat.

## Shipping a change

Merging to `main` does **not** deploy. In Railway, open the `uplumo` service
and deploy the latest commit of `main` (or reconnect the source to branch
`main`, which starts a deploy). Changing a variable also redeploys.

After a deploy, open `https://uplumo.com/api/health` and the page you changed.

## 1. Create the project

1. In Railway, create a project and add **PostgreSQL** and **Redis**.
2. Add a service from the GitHub repo `ToufiqQureshi/uplumo`, branch `main`.
   Railway reads `railway.json`: it builds the Dockerfile and checks
   `/api/health`.

## 2. Set the variables

Required, or the server does not start:

| Variable             | Value                                                                                              |
| -------------------- | -------------------------------------------------------------------------------------------------- |
| `DATABASE_URL`       | `${{Postgres.DATABASE_URL}}`                                                                       |
| `REDIS_URL`          | `${{Redis.REDIS_URL}}`                                                                             |
| `BETTER_AUTH_SECRET` | 32 or more random characters. Never change it later: it signs sessions and encrypts Google tokens. |
| `PUBLIC_URL`         | `https://uplumo.com`                                                                               |
| `BETTER_AUTH_URL`    | `https://uplumo.com`                                                                               |

Needed before real users sign up:

| Variables                                                | Without them                                                                                                                                     |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `LOOPS_API_KEY` and the three `LOOPS_TRANSACTIONAL_*_ID` | No verification or reset email: nobody can finish sign-up. The key, the verify ID and the reset ID must all be set, or none of the three works. |
| `TURNSTILE_SECRET_KEY`, `TURNSTILE_SITE_KEY`             | No bot check on sign-up.                                                                                                                         |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`               | No Google sign-in, Search Console or Analytics.                                                                                                  |
| The four `RAZORPAY_*`                                    | No paid plan: every billing endpoint answers `503`.                                                                                              |
| `BILLING_MONTHLY_CREDITS_USD`                            | A paying workspace is granted no credits, and the assistant and AI writing are charged against credits.                                          |
| `DATAFORSEO_API_KEY`                                     | No keywords, rank tracking, backlinks or AI visibility (`503`).                                                                                  |
| `OPENROUTER_API_KEY`, `OPENROUTER_MODEL`                 | No AI assistant or AI writing.                                                                                                                   |

Optional: `GOOGLE_TOKEN_ENCRYPTION_KEY` (base64, 32 bytes) encrypts Google
tokens with its own key instead of one derived from `BETTER_AUTH_SECRET`.
`TRUSTED_PROXY_CIDRS` and `OPENAI_APPS_CHALLENGE` are described in
`backend/README.md`.

`TURNSTILE_SITE_KEY`, `POSTHOG_PUBLIC_KEY` and `POSTHOG_HOST` are baked into
the frontend at build time, so redeploy after changing them.

The server refuses to start on a live `https` site when:

- `BYPASS_EMAIL_VERIFICATION` is `true`, or
- `DATAFORSEO_API_KEY` or `OPENROUTER_API_KEY` is set without the `RAZORPAY_*`
  variables. Without billing nothing limits usage, and every lookup is charged
  to our provider accounts.

## 3. Point the outside services at the site

| Service    | Setting                                                                                                                                                                                                      |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Domain     | In Railway add `uplumo.com` as a custom domain, then add the DNS record it shows in Cloudflare.                                                                                                              |
| Cloudflare | SSL mode Full. One redirect rule sends `www.uplumo.com` to `uplumo.com`. Email Routing forwards `support@uplumo.com` to the owner's mailbox: every public page links to that address.                         |
| Google     | Authorised redirect URIs: `https://uplumo.com/api/auth/callback/google` (sign-in), `https://uplumo.com/api/gsc/oauth/callback` (Search Console) and `https://uplumo.com/api/ga4/oauth/callback` (Analytics). Enable the Search Console API and the Analytics Data and Admin APIs in the same Google Cloud project. |
| Loops      | Verify the sending domain `mail.uplumo.com` and publish the three transactional templates. Template variables and checks are in `backend/internal/email/README.md`.                                          |

### Razorpay

Billing uses Razorpay **Subscriptions**. Do this once in Test Mode, then again
in Live Mode:

1. Create a monthly plan whose amount equals `ProPriceINR` in
   `backend/internal/site/site.go` (₹1,499 today). Its id (`plan_...`) is
   `RAZORPAY_PLAN_ID_PRO`.
2. Add a webhook to `https://uplumo.com/webhooks/razorpay` with a secret of
   your own; the same value is `RAZORPAY_WEBHOOK_SECRET`. Subscribe it to
   `subscription.activated`, `.charged`, `.cancelled`, `.halted` and
   `.completed`.
3. Generate an API key: `RAZORPAY_KEY_ID` and `RAZORPAY_KEY_SECRET`.
4. Set all four variables. A test key works only with a test plan and a test
   webhook; never mix modes.

## 4. Check the live site

1. `https://uplumo.com/api/health` returns `{"status":"ok"}`.
2. The landing, pricing, about, contact, terms, privacy, refunds and shipping pages open, and each shows `support@uplumo.com` as text.
3. Sign up with a real address, receive the verification email, reach the
   dashboard.
4. Buy Pro with a Razorpay test card, see the workspace switch to Pro, then
   cancel it from Billing.
5. Ask for a password reset and finish it.
6. Invite a teammate and open the invitation email.

## Rolling back

In Railway, open the last good deployment and choose **Redeploy**. Migrations
only move forward, so a rollback keeps the newer database schema.

## Erasing an account

The image also carries `/erase-user`. Its use is in
`backend/internal/gdpr/README.md`.
