# Content editor

The editor stores each article as Markdown in a project-scoped draft. The title,
target topic, URL slug and search description are saved with the article. SEO
and answer-readiness checks are local editorial guidance; they do not query
Google or predict rankings.

## Writing flow

- Write or edit Markdown in the article field, then switch to Preview to check
  headings, links, lists and embedded images.
- The Image action inserts a publicly reachable HTTPS or HTTP image URL and
  requires descriptive alt text. Binary uploads are not available because this
  feature has no media-storage endpoint yet.
- The AI assistant is optional and uses organization AI credits. Generated
  claims should be checked before they are used.
- Save, reopen and delete operations are project-scoped. Draft lists and draft
  details use TanStack Query; opening the same draft again while its cache is
  fresh reuses the cached response. The shared query client keeps in-memory
  query data for five minutes. A full browser reload clears that cache.

## Rules and limits

- Do not describe readiness checks as Google's ranking algorithm or a ranking
  guarantee. Do not require a target keyword density, fixed word count, or
  fixed title/meta length.
- Keep article content below the API's 150,000-character editor limit.
- Do not persist drafts in browser storage; drafts may contain customer data.
