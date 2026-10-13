# Analytics

Uplumo Analytics: a tiny cookie-free tracker (`/t.js`, source in
`internal/httpapi/tracker.js`) that counts pageviews and custom events for a
project's site, and the summary the Analytics page shows.

## Flow

1. The Analytics page calls `POST /api/v1/projects/{id}/analytics/site`,
   which creates the project's site key once and returns the snippet:
   `<script defer data-site="KEY" src="https://uplumo.com/t.js"></script>`.
2. The tracker posts `{k, u, r, w}` to `POST /collect` for each pageview,
   including single-page-app navigation.
3. **Custom events.** Any element with `data-uplumo-event="Name"` is tracked
   when clicked, with no code (`<button data-uplumo-event="Signup">`). Code
   can call `window.uplumo("Name")`. The tracker posts the same body plus
   `n` (the event name, trimmed, 1 to 64 characters).
4. `GET .../analytics/summary` returns visitors, pageviews, pages, channels,
   AI sources, referrers, devices and `events` (custom events by count and
   visitors, top 50).

## Tables

- `go_analytics_sites`: one site key per project.
- `go_analytics_events`: one row per pageview.
- `go_analytics_custom_events`: one row per custom event. Kept apart so
  events never inflate pageviews or visitors.

## Rules that must never break

- No cookies, no stored IP. The visitor id is SHA-256 of a daily salt that
  lives only in Redis, the site key, the IP and the user agent; it cannot be
  followed across days.
- `/collect` is open to every origin and needs no CORS preflight (text/plain
  body). It drops bots, refuses a page outside the project's domain
  (`foreign_host`) and rate-limits per IP and site.
- A custom event goes through the same bot, domain and rate checks as a
  pageview.

## Production notes

If the Analytics page shows no visits, check the Railway HTTP logs for
`/t.js` and `/collect`: no requests means the snippet is not on the site.
