# Content authors

Project author profiles supply the visible WordPress byline and the optional
`BlogPosting.author` node. They are editorial identities, separate from Uplumo
login accounts. A project can have several profiles; a draft selects one.

The three `/api/v1/projects/{projectId}/content/authors/*` routes use session
and project access middleware. The list, save and delete operations are scoped
to that project. Names are required, bios are capped at 300 characters, and
optional photo and profile links must be safe HTTP(S) URLs. Rendering escapes
author text before it enters Markdown or JSON-LD.

Migration `00024_content_authors.sql` adds `go_content_authors`, plus nullable
`author_id` and `last_reviewed_at` on drafts. Deleting a profile clears the
draft's author reference through `ON DELETE SET NULL`; the draft survives.
The selected author must belong to the same project as the draft. A missing
profile never blocks publication: its byline and author schema node are
omitted.
