# Content editor

The editor stores each article as Markdown in a project-scoped draft. The title,
target topic, URL slug and search description are saved with the article. SEO
and answer-readiness checks are local editorial guidance; they do not query
Google or predict rankings.

## Scoring in the editor

- Scores are deterministic and computed in the browser after a 300 ms pause in
  typing. Scoring does not make network requests. A `ContentBrief` is optional
  cached input; until one is available, the editor says it is using only
  brief-independent checks. Fixtures used by tests are never loaded into the
  product UI.
- With a brief, SEO uses the approved 100-point weights: term coverage (30),
  term ranges (10), topic placement (15), depth (10), brief subtopics (10),
  list/table (5), sources and internal links (8), readability (7), and image
  alt text (5). Terms are selected by competitor-domain prevalence. Depth uses
  the 25th–75th percentile of available competitor article lengths, with a
  bounded fallback when there are no samples.
- Without a brief, SEO displays only local checks, scaled against the points
  available in that mode. It does not imply competitor coverage or article
  depth has been checked. AEO remains separately scored against its direct
  answer, definition, question coverage, structure, sources, FAQ, entities and
  freshness checks.
- Readability is scored only when the draft contains no Devanagari script and
  fewer than 5% common Hindi/Hinglish markers. Other language drafts receive no
  readability penalty. Image alt text remains checked against the existing
  five-image editor limit.
- SEO and AEO are capped at 40 and publishing is blocked when the exact target
  topic appears at least eight times, exceeds twice the larger of its brief
  maximum and 2% of body words, and exceeds 4% of body words. This check works
  without a brief too. Eight-word phrase overlap with a competitor is considered
  copied when at least four shingles match and at least 20% of the draft's
  shingles overlap. That caps SEO at 30. SEO and AEO drafts under 100 words are
  also capped at 30. These are editorial safeguards, not search-engine rules.
- Brief term coverage is partial below the minimum (`uses / min`), full from the
  minimum through the maximum, and overuse is handled by the separate term-range
  check. A missing image does not earn alt-text points.
- Title and description lengths are hints only. No keyword-density target is
  used. The slug follows the title until the writer edits the slug. Unsaved
  browser navigation warns the writer; drafts remain saved only through the
  server API and are not stored in browser storage.

## Writing flow

- Write or edit Markdown in the article field, then switch to Preview to check
  headings, links, lists and embedded images.
- The Image action accepts a public HTTPS or HTTP image URL, or a PNG/JPEG file
  from the device. File uploads are resized and compressed to WebP (2,400 px per
  side, 8 MP and 1 MB maximum) before they are stored in the public Supabase
  Storage bucket. An article can contain at most five Markdown images; a
  workspace can upload at most 20 images per hour. Add descriptive alt text.
  Public bucket images are accessible to anyone with the URL, so do not upload
  private images.
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
- Configure `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, and
  `SUPABASE_CONTENT_IMAGE_BUCKET` on the backend. Create that bucket as public
  and restrict its allowed MIME type to `image/webp` with a 1 MB object-size
  limit in Supabase. The service-role key must never be exposed to the browser.
- Do not persist drafts in browser storage; drafts may contain customer data.
