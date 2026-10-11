# Outbound email

The Go email package owns provider calls used by Go features. Its Loops client
sends transactional messages with a 10-second request deadline and never adds
transactional recipients to the marketing audience. Provider errors are
returned without logging response bodies, which may contain recipient data.

Team invitations use this client today. TypeScript still owns signup contacts,
verification and password-reset mail, plus billing lifecycle contacts/events;
those callers stay until their authentication and billing flows move to Go.

`LOOPS_API_KEY` and `LOOPS_TRANSACTIONAL_INVITATION_ID` configure invitations.
Missing configuration or a provider failure maps to an unavailable email
provider at the feature boundary.
