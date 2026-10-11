// Package emailauth implements email and password authentication over the
// legacy Better Auth tables and HTTP contract: sign-up, sign-in, email
// verification, password reset and profile update.
//
// The routes, JSON bodies, error codes, cookies and password hashes match
// Better Auth 1.6, so the React client and every account already in the database
// keep working unchanged.
package emailauth
