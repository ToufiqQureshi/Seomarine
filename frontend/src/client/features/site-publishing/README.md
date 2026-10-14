# site-publishing

Screens for connecting a project's WordPress site and publishing content-editor
drafts to it. Rules, limits and errors live in
`backend/internal/sitepublish/README.md`.

- `WebsiteConnectionCard`: connect form (site, username, application
  password) or the connected site with Disconnect. Shown on
  Settings → Integrations (`#website`) and in onboarding
  (`onboarding/WebsiteOnboardingStep.tsx`, the step after Search Console).
  A Free workspace sees a link to the plans instead of a form the server
  would refuse; members who are not owner or admin are told to ask one.
- `PublishToSite`: in the content editor's sidebar. Publishes the **saved**
  draft, so it waits while there are unsaved changes. "Send as draft" makes a
  WordPress draft; "Publish now" makes it live; both update the same post
  the second time.
- The application password is only held in the form until it is sent; it is
  never cached or read back.
- Errors from the customer's own site (`site_unreachable`, `site_error`, both 502) keep the server's message, so the customer sees what WordPress said.
