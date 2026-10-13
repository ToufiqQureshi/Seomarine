# Outbound email

The Go email package owns provider calls used by Go features. Its Loops client
sends transactional messages with a 10-second request deadline and never adds
transactional recipients to the marketing audience. Provider errors are
returned without logging response bodies, which may contain recipient data.

## What Loops does for Uplumo

| Use                | Sent by                         | When                                         |
| ------------------ | ------------------------------- | -------------------------------------------- |
| Verify email       | `internal/emailauth`            | Email and password sign-up, and "resend"     |
| Password reset     | `internal/emailauth`            | "Forgot password"                            |
| Team invitation    | `internal/team`                 | An owner or admin invites a teammate         |
| Sign-up contact    | `internal/emailauth`            | Every new user is saved as a Loops contact   |

Loops does not handle incoming mail, payment receipts (Razorpay sends those)
or scheduled reports. Google sign-in sends no verification or reset mail.

## Variables

| Variable                                | Used for        |
| --------------------------------------- | --------------- |
| `LOOPS_API_KEY`                         | Every call      |
| `LOOPS_TRANSACTIONAL_VERIFY_EMAIL_ID`   | Verify email    |
| `LOOPS_TRANSACTIONAL_RESET_PASSWORD_ID` | Password reset  |
| `LOOPS_TRANSACTIONAL_INVITATION_ID`     | Team invitation |

**Verification and reset switch on together.** `cmd/server` builds the auth
mailer only when the API key, the verify ID and the reset ID are all set. With
any one missing, sign-up still answers 200 but sends nothing, and
`/api/auth/send-verification-email` answers `VERIFICATION_EMAIL_NOT_ENABLED`.
That response is the quickest way to tell the mailer is off.

Invitations need only the API key and the invitation ID. Without them the
invite endpoint reports an unavailable email provider.

## Templates

Each ID is a transactional email published in the Loops dashboard. Loops
rejects a send when the template uses a data variable the request does not
carry, so a template may use only the names below, spelled exactly.

| Template        | Data variables                                                            | Button link  |
| --------------- | ------------------------------------------------------------------------- | ------------ |
| Verify email    | `confirmationUrl`, `appName`                                              | `confirmationUrl` |
| Password reset  | `resetUrl`, `appName`                                                     | `resetUrl`   |
| Team invitation | `inviteUrl`, `organizationName`, `inviterName`, `inviterEmail`, `appName` | `inviteUrl`  |

A template made by duplicating another keeps the old button link. Check the
link variable before publishing.

## Sending domain

Mail is sent from `mail.uplumo.com`, not the root domain. Its seven DNS
records live in Cloudflare under that subdomain: an MX and an SPF record on
`envelope.mail`, three DKIM CNAMEs (not proxied), `_dmarc.mail` and
`_loops-verification.mail`. The Loops dashboard can show a record as missing
for hours after it is public; `nslookup` against `1.1.1.1` is the check.

The root domain's MX records belong to Cloudflare Email Routing, which
forwards `support@uplumo.com` to the owner's inbox. The two do not overlap.

## Checking a live deploy

1. `POST /api/auth/send-verification-email` with a JSON `email` and
   `callbackURL`. `VERIFICATION_EMAIL_NOT_ENABLED` means a variable is missing.
2. A 200 from that endpoint for an unverified address means Loops accepted
   the message; a failed send returns an error. A failed reset send is only
   logged, as `send password reset email failed`, so that the answer does not
   reveal whether the address has an account.
3. Whether Loops delivered a message is on the template's Metrics tab in the
   Loops dashboard.
