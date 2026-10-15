# Reports and share links: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Creates report templates, lists/renders project reports and manages optional public share links.

## Decisions to preserve

- Public sharing is opt-in; never expose a report through an unguessable-but-unchecked ID alone. _Why:_ Reports may contain client data, so only an explicit share state may grant public access.
- Keep organization/project authorization on private routes and revocation effective for shared links. _Why:_ A report ID is not proof of membership, and revoked public access must not remain usable.
- Report generation should use stored data unless a clearly labeled refresh action is requested. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.
- Manual report content is plain text escaped into HTML; preserve character and URL limits to avoid stored markup injection and oversized report bodies. _Why:_ Report fields are user-controlled and later rendered or shared, so escaping and limits protect recipients and storage.
- Work-log sharing is opt-in and happens after the report save; a share failure must not roll back or hide a successfully saved report. _Why:_ Saving customer work and publishing a public link are distinct actions with different privacy consequences.

## Verification pointers

- `reportApi.test.ts`
