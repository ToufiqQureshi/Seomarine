# Content editor: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. The editor loads a selected project and draft, then keeps editing state in the editor components.
2. Brief, AI assistant, authors, link suggestions, image upload and draft save each use their own API action; link suggestions run only when requested.
3. Go validates project access and inputs, stores drafts/authors, optionally calls configured providers, and returns a structured response.
4. A publish preview is separate from publishing; publication occurs only through the explicit site-publishing action.

## Implementation map

- `ContentEditorPage.tsx`
- `contentEditorApi.ts`
- `WriterAssistant.tsx`
- `linkSuggestions.ts`
- `backend/internal/contenteditor`
- `backend/internal/contentauthors`
- `backend/internal/linksuggest`
- `backend/internal/sitepublish`

## API and service dependencies

Go packages: `contenteditor`, `contentauthors`, `linksuggest` and `sitepublish`; costs depend on the specific explicit action.

## Cost, security and failure boundaries

- AI writing/brief calls may spend credits or provider money; keep plan, credit and output-size guards server-side.
- Do not auto-save or publish unreviewed generated text to a customer site.
- Uploaded images and generated HTML must be constrained to allowed storage/content types.
- Link suggestions use project-owned GSC/audit data and must never cross project boundaries.
