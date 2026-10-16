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
  five-image editor limit. When there are no images, its check is marked not
  applicable and removed from the score; the remaining SEO points are scaled
  to the full score.
- SEO and AEO are capped at 40 and publishing is blocked when the exact target
  topic appears at least eight times, exceeds twice the larger of its brief
  maximum and 2% of body words, and exceeds 4% of body words. This check works
  without a brief too. Eight-word phrase overlap with a competitor is considered
  copied when at least four shingles match and at least 20% of the draft's
  shingles overlap. That caps SEO at 30. SEO and AEO drafts under 100 words are
  also capped at 30. When multiple caps apply, the lowest score cap wins, while
  any stuffing trigger still blocks publishing. These are editorial safeguards,
  not search-engine rules.
- Brief term coverage is partial below the minimum (`uses / min`), full from the
  minimum through the maximum, and overuse is handled by the separate term-range
  check.
- Title and description lengths are hints only. No keyword-density target is
  used. The slug follows the title until the writer edits the slug. Unsaved
  browser navigation warns the writer; drafts remain saved only through the
  server API and are not stored in browser storage.

## Writing flow

- The article body is a visual editor (TipTap 3, `visual-editor/`): toolbar,
  a toolbar over selected text (bold, italic, link, H2, H3), a "+" button and
  "/" menu on empty lines (headings, lists, image, table, quote, FAQ), and
  pasting from Google Docs or Word keeps headings, lists and links. "Preview as
  on website" opens the article at desktop or phone width. A "Markdown" toggle
  shows the raw Markdown for writers who prefer it.
- The draft is still stored as Markdown. `@tiptap/markdown` serializes every
  change, so scoring, saving and WordPress publishing are unchanged. The editor
  may only offer what Markdown can hold: no underline, colours or alignment.
  `visual-editor/extensions.test.ts` round-trips every construct (H1–H4, marks,
  links, lists, quote, image alt text, tables, Hindi text) and checks the score
  is identical before and after; a schema change that drops one would delete it
  from customers' articles.
- Links: `normalizeLinkUrl` adds https:// to a bare domain, keeps internal
  paths (`/services`, `#faq`) and `mailto:`, and refuses `javascript:`, `data:`
  and `user:pass@` links, because the article is published to the customer's
  site as written.
- The Image action, "/" Image, or pasting or dropping a PNG/JPEG opens the
  image panel; a pasted or dropped file is preselected and the writer only adds
  the description. It accepts a public HTTPS or HTTP image URL, or a PNG/JPEG file
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

## Search brief

- The Brief tab calls `POST /api/v1/projects/{projectId}/content/brief` with
  `{"keyword", "refresh"}` (backend: `contenteditor/brief*.go`). Only when the
  writer presses the button: a brief costs credits, so it never runs on its
  own, and it is never fetched while typing.
- One brief = a depth-30 Google SERP plus up to 8 competitor pages read with
  DataForSEO content parsing, **one page per call** (the live endpoint takes
  one task). Unreadable pages are skipped; fewer than 3 readable pages means
  no brief and no charge. Worst case $0.0072 (about 9.2 credits); the customer
  pays what DataForSEO reports, never more. Any overage is Uplumo's.
- The cache (`go_content_brief_cache`) is shared by every workspace for 30
  days, keyed by keyword, location and language. A cache hit is free; two
  requests for the same key at once make one provider call (advisory lock).
  The customer's own domain is filtered out when the brief is read, never in
  the key. "Refresh" rebuilds it and charges the refreshing workspace.
- Plans: Basic and up (`plans.Keywords` gate). It fails closed: no plan
  service or a failing plan check means nothing is fetched.
- The brief gives terms (phrases most pages use, ranked by how many pages use
  them, with a 25th–75th percentile use range), subtopics (phrases several
  pages use in headings), People Also Ask questions, and each page's headings,
  word count and main text (20,000 characters max, for depth and the copy
  check). A brief applies only to the topic it was built for; changing the
  topic stops it applying.
- AI writing actions get the brief's terms, questions and headings, never the
  competitor text, to stay inside the 15-credit per-action ceiling. The AI
  writer always uses `openai/gpt-4.1-mini`; `OPENROUTER_MODEL` configures SAM
  only.

## Free publishing guidance

- A draft can select a project author profile and record its last review date.
  The AEO freshness check uses that date, through the same calendar day twelve
  months later.
- The Publish tab previews `BlogPosting` and `FAQPage` JSON-LD from the last
  saved draft. Each block has its own switch. FAQ requires a visible section
  with matching questions and answers. WordPress may omit schema if the account
  cannot keep script tags in post content; the publish result explains why.
- **Find internal links** reads the project's Search Console or audit pages on
  request. Typing never starts this lookup. **Link it** applies one TipTap link
  command to the matching anchor after URL validation. No credits are spent.

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
- Drafts autosave after three seconds of inactivity. The header and search
  preview use the page title, then the first H1, and show the workspace's
  connected domain in the preview URL.
