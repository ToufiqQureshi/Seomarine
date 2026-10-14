# Blog posts

Each `.md` file in this folder (except this one) is one post at
`https://uplumo.com/blog/<file-name>`. Posts are rendered when the server
starts, listed on `/blog` newest first, added to `/sitemap.xml` and given
`BlogPosting` structured data. A post with a mistake stops the server from
starting, and the tests fail first, so a broken post never reaches the site.

## Add a post

1. Name the file after the URL you want: lower-case words joined by hyphens,
   for example `what-is-aeo.md` → `/blog/what-is-aeo`. Never rename a
   published file; the old URL would break.
2. Start it with this front matter, then write Markdown below it:

   ```
   ---
   title: What is AEO? Answer engine optimisation explained
   description: How to get your pages quoted by ChatGPT, Perplexity and Google AI Overviews, in five steps any small business can follow.
   date: 2026-10-20
   ---

   Your post starts here. Use ## for sections and ### below them.
   ```

   | Field         | Required | Rule                                                                   |
   | ------------- | -------- | ---------------------------------------------------------------------- |
   | `title`       | yes      | Up to 70 characters (longer is cut off in Google). Main keyword first. |
   | `description` | yes      | 50 to 160 characters. Shown under the title in Google.                 |
   | `date`        | yes      | `YYYY-MM-DD`, the publish date.                                        |
   | `updated`     | no       | `YYYY-MM-DD`, set it when you change a post a lot.                     |
   | `author`      | no       | Defaults to "Uplumo team".                                             |
   | `draft`       | no       | `true` keeps the post off the site.                                    |

3. Images: put them in `backend/internal/site/static/blog/` and write
   `![What the image shows](/site/blog/file-name.webp)`. Only images on
   uplumo.com load (the pages' security policy blocks other sites).
4. Raw HTML in a post is dropped. Use Markdown only.
5. Commit on a branch, open a PR, merge, then deploy (`docs/DEPLOY.md`).
   Editing on github.com works: open this folder, "Add file" → "Create new
   file".

## Writing posts that rank

- **One search intent per post.** Pick the question a reader typed, put that
  keyword in the title, the first paragraph and one `##` heading.
- **Answer first.** The first two or three sentences answer the question
  directly; AI assistants and featured snippets quote that part.
- **Structure:** `##` sections a reader can scan, short paragraphs, lists
  and tables for steps and comparisons, a short FAQ at the end.
- **Show it is real:** your own numbers, screenshots and examples from
  Indian businesses. Thin, generic or copied text does not rank.
- **Link** to 2–3 other Uplumo posts or pages, and to sources you cite.
- **Length:** as long as the answer needs. Most guides land at 1,000–2,000
  words.
- **Publish steadily** (one or two posts a week beats ten at once) and
  update old posts with `updated:` instead of writing near-duplicates.
- After a post is live, ask Google to index it in Search Console (URL
  inspection → "Request indexing"). The sitemap is already submitted at
  `https://uplumo.com/sitemap.xml`.
