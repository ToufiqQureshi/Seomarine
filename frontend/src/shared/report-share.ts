// Shared path helper for public report links. Token generation and validation
// live in the Go reports service.

/** The public path for a token. One definition, used by both routes and the app. */
export const sharePath = (token: string) => `/s/${token}`;
