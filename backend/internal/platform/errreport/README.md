# Error reporting

Sends every error-level log line to Sentry, so a failure on production reaches
a person instead of waiting in the Railway logs.

## Flow

`cmd/server` builds a `Reporter` from `SENTRY_DSN` and wraps the logger with
`Reporter.Handler`. From then on:

- A record at error level goes to the normal log **and** becomes a Sentry
  event. Info and warn records only go to the log.
- A handler panic is reported too, because `httpx.Recover` logs it at error
  level with its stack.
- When the server stops with an error, that error is logged, reported, and
  flushed (two seconds) before the process exits.

The log message is the Sentry issue title and its fingerprint: messages are
fixed strings (`"billing cancel"`), so one message is one issue however the
error text varies. The record's attributes, including `err`, are in the event's
`log` context as text.

## Settings

| Variable                 | Meaning                                                      |
| ------------------------ | ------------------------------------------------------------ |
| `SENTRY_DSN`             | The Sentry project address. Unset turns reporting off.       |
| `RAILWAY_GIT_COMMIT_SHA` | Set by Railway. Sent as the release, so an issue names the deploy that caused it. |

The environment is `production` on a live `https` site and `development`
anywhere else.

## Rules that must never break

- Reporting never changes what is logged or what a request returns. A `nil`
  `Reporter` (no DSN) is a no-op.
- **Whatever is logged at error level leaves our servers.** Never log a
  password, token, cookie or API key at any level, and keep personal data out
  of error logs.
- Only errors are sent. Tracing, Sentry Logs and metrics are off: they are
  billed by volume and send far more data. Turning one on is an owner decision.

## Not built

- Browser errors. The React app does not report to Sentry.
- Request details (URL, user) on an event, beyond what the log line carries.
