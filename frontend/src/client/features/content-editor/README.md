# Content editor

The editor stores each article as Markdown in a project-scoped draft. The title,
target topic, URL slug and search description are saved with the article. SEO
and answer-readiness checks are local editorial guidance; they do not query
Google or predict rankings.

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
