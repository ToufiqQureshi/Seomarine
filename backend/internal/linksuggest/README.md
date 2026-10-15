# Internal link suggestions

The editor requests up to ten links to the project's own pages using
`POST /api/v1/projects/{projectId}/content/link-suggestions`. The route uses
session and project access middleware. The UI calls it only when the writer
selects **Find internal links**, so typing does not consume Search Console
quota.

The first source is pages with impressions in the project's last 90 days of
Search Console data, including top queries and traffic. When Search Console
has no pages, the latest site audit's crawled pages are used. Neither source
makes a paid provider call. Results are ranked by clicks, then impressions;
the draft's own slug is excluded. Text scanning considers at most 50,000 runes,
2,000 segments and 200 source pages. A full editor draft remains bounded.

Results are cached in Redis for ten minutes by project and draft-input hash.
Redis failure falls back to computing the free result. A source outage falls
back to the next source; with no available pages the API returns an empty list.
The editor checks the anchor still exists and applies a safe URL through the
TipTap link command as one undoable edit.
