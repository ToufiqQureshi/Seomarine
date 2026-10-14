# sitepublish

Connects a project to the customer's own website and publishes content-editor
drafts to it. WordPress is the only platform today; the routes take a
`platform` so Shopify, Wix, Webflow or SFTP can be added without changing the
contract.

## Flow

1. **Connect** (`POST /api/v1/projects/{projectId}/site-connection/save`,
   owner or admin, Basic and above). The customer gives the site address, a
   WordPress username and an **application password** (WordPress → Users →
   Profile; built in since WordPress 5.6). Uplumo:
   - finds the REST API at `/wp-json/`, or `/?rest_route=/` for sites on
     plain permalinks, following up to 3 https redirects (`example.com` →
     `www.example.com`);
   - refuses a site whose REST index does not advertise application passwords
     (often turned off by a security plugin);
   - signs in to `/wp/v2/users/me` and requires `publish_posts` and
     `upload_files`;
   - only then stores the connection, the password encrypted with AES-GCM.
2. **Publish** (`POST .../content/drafts/{draftId}/publish`, any member, Basic
   and above), `{"status": "draft" | "publish"}`:
   - each image in the draft's Markdown is downloaded and uploaded to the
     site's media library, then the article points at the site's copy; the
     first image becomes the featured image;
   - the Markdown is rendered to HTML (goldmark, GitHub-flavoured);
   - the post is created, or the earlier post updated.
3. `.../site-connection/get`, `.../site-connection/delete` (owner or admin)
   and `.../content/drafts/{draftId}/publication/get` read and remove.

## Tables

- `go_site_connections`: one row per project. `secret_ciphertext` is the
  application password, encrypted.
- `go_site_publications`: one row per published draft: the remote post id and
  link, and `media`, the images already copied (`{"<draft url>": {"id",
  "url"}}`). Deleted with the draft.

## Rules that must never break

- **The password never leaves the server.** It is encrypted at rest with a key
  derived from `BETTER_AUTH_SECRET` (rotating that secret means every
  customer reconnects) and no route returns it.
- **A signed-in request never follows a redirect**, so the password only goes
  to the address that was checked. Only password-free discovery and image
  downloads follow redirects, and only to https.
- **Every outbound request goes through `audit.Guard`**: no private, loopback
  or metadata addresses, checked at dial time against DNS rebinding.
- **Raw HTML in a draft is dropped and `javascript:` links are not rendered**,
  so an article cannot carry a script onto the customer's site.
- **Nothing is stored unless WordPress accepted the password.**
- **Publishing again updates the same post**; images already copied are not
  uploaded twice. A post deleted on the site is created again; after
  connecting a different site, publishing starts fresh there.

## Limits

- At most 10 images per article, 5 MB each; PNG, JPEG, GIF and WebP only (no
  SVG: it can carry script).
- A publish has 100 seconds (the handler lifts the server's 30 second write
  deadline). Connecting has 25 seconds.
- Only https sites. WordPress itself refuses application passwords over plain
  http.

## Edge cases

- **Hosting strips the `Authorization` header** (some Apache/CGI setups):
  WordPress answers 401 and the customer is told to ask the host to allow it.
- **A publish that times out after images were uploaded** leaves those images
  in the media library; the next publish uploads them again because the
  mapping is saved with the post. Rare with 10 images and a 100 second budget.
- **The meta description goes to the WordPress excerpt.** Yoast and Rank Math
  keep their own description fields, which the REST API does not write
  without a plugin.
- A workspace that stops paying keeps its connection and can still read and
  disconnect; it cannot publish.
