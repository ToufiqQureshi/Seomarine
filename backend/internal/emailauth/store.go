package emailauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// querier is satisfied by *pgxpool.Pool and pgx.Tx alike, so every statement
// runs the same way inside or outside a transaction.
type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// User is a row of the Better Auth user table.
type User struct {
	ID                string
	Name              string
	Email             string
	EmailVerified     bool
	Image             *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	AnalyticsOptedOut *bool
}

// Session is a row of the Better Auth session table.
type Session struct {
	ID                   string
	Token                string
	UserID               string
	ExpiresAt            time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	IPAddress            *string
	UserAgent            *string
	ActiveOrganizationID *string
}

const userColumns = `id, name, email, email_verified, image, created_at, updated_at, analytics_opted_out`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.EmailVerified, &u.Image, &u.CreatedAt, &u.UpdatedAt, &u.AnalyticsOptedOut)
	return u, err
}

// userByEmail finds a user by email, which is stored lowercased.
func userByEmail(ctx context.Context, q querier, email string) (User, bool, error) {
	u, err := scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM "user" WHERE email = $1`, email))
	return found(u, err, "user by email")
}

func userByID(ctx context.Context, q querier, id string) (User, bool, error) {
	u, err := scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM "user" WHERE id = $1`, id))
	return found(u, err, "user by id")
}

func found[T any](value T, err error, what string) (T, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return value, false, nil
	}
	if err != nil {
		return value, false, fmt.Errorf("find %s: %w", what, err)
	}
	return value, true, nil
}

// credentialHash returns the user's password hash, if they have a credential
// account with one.
func credentialHash(ctx context.Context, q querier, userID string) (string, bool, error) {
	var hash *string
	err := q.QueryRow(ctx, `SELECT password FROM account WHERE user_id = $1 AND provider_id = 'credential' LIMIT 1`, userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && hash == nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find credential account: %w", err)
	}
	return *hash, true, nil
}

func insertUser(ctx context.Context, q querier, u User) error {
	if _, err := q.Exec(ctx, `INSERT INTO "user" (id, name, email, email_verified, image, created_at, updated_at, analytics_opted_out)
		VALUES ($1, $2, $3, $4, $5, $6, $6, $7)`, u.ID, u.Name, u.Email, u.EmailVerified, u.Image, u.CreatedAt, u.AnalyticsOptedOut); err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func insertCredentialAccount(ctx context.Context, q querier, id, userID, hash string, now time.Time) error {
	if _, err := q.Exec(ctx, `INSERT INTO account (id, account_id, provider_id, user_id, password, created_at, updated_at)
		VALUES ($1, $2, 'credential', $2, $3, $4, $4)`, id, userID, hash, now); err != nil {
		return fmt.Errorf("insert credential account: %w", err)
	}
	return nil
}

// setPassword replaces the user's password, creating the credential account for
// a user who so far only signed in with a provider.
func setPassword(ctx context.Context, q querier, accountID, userID, hash string, now time.Time) error {
	tag, err := q.Exec(ctx, `UPDATE account SET password = $2, updated_at = $3 WHERE user_id = $1 AND provider_id = 'credential'`, userID, hash, now)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	return insertCredentialAccount(ctx, q, accountID, userID, hash, now)
}

func insertSession(ctx context.Context, q querier, s Session) error {
	if _, err := q.Exec(ctx, `INSERT INTO session (id, expires_at, token, created_at, updated_at, ip_address, user_agent, user_id, active_organization_id)
		VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8)`,
		s.ID, s.ExpiresAt, s.Token, s.CreatedAt, s.IPAddress, s.UserAgent, s.UserID, s.ActiveOrganizationID); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func deleteUserSessions(ctx context.Context, q querier, userID string) error {
	if _, err := q.Exec(ctx, `DELETE FROM session WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func markEmailVerified(ctx context.Context, q querier, email string, now time.Time) error {
	if _, err := q.Exec(ctx, `UPDATE "user" SET email_verified = true, updated_at = $2 WHERE email = $1`, email, now); err != nil {
		return fmt.Errorf("mark email verified: %w", err)
	}
	return nil
}

func insertVerification(ctx context.Context, q querier, id, identifier, value string, expiresAt, now time.Time) error {
	if _, err := q.Exec(ctx, `INSERT INTO verification (id, identifier, value, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)`, id, identifier, value, expiresAt, now); err != nil {
		return fmt.Errorf("insert verification: %w", err)
	}
	return nil
}

// peekVerification reads an unexpired verification value without using it up.
func peekVerification(ctx context.Context, q querier, identifier string, now time.Time) (string, bool, error) {
	var value string
	err := q.QueryRow(ctx, `SELECT value FROM verification WHERE identifier = $1 AND expires_at > $2
		ORDER BY created_at DESC LIMIT 1`, identifier, now).Scan(&value)
	return found(value, err, "verification")
}

// consumeVerification deletes the verification row and returns its value if it
// had not expired. A token is therefore single-use even when two requests race:
// only one DELETE gets the row.
func consumeVerification(ctx context.Context, q querier, identifier string, now time.Time) (string, bool, error) {
	var (
		value     string
		expiresAt time.Time
	)
	err := q.QueryRow(ctx, `DELETE FROM verification WHERE id = (
			SELECT id FROM verification WHERE identifier = $1 ORDER BY created_at DESC LIMIT 1
		) RETURNING value, expires_at`, identifier).Scan(&value, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("consume verification: %w", err)
	}
	if !expiresAt.After(now) {
		return "", false, nil
	}
	return value, true, nil
}

// updateProfile applies only the fields that are set.
func updateProfile(ctx context.Context, q querier, userID string, name, image *string, analyticsOptedOut *bool, now time.Time) error {
	if _, err := q.Exec(ctx, `UPDATE "user" SET
			name = COALESCE($2, name),
			image = COALESCE($3, image),
			analytics_opted_out = COALESCE($4, analytics_opted_out),
			updated_at = $5
		WHERE id = $1`, userID, name, image, analyticsOptedOut, now); err != nil {
		return fmt.Errorf("update user profile: %w", err)
	}
	return nil
}

// isUniqueViolation reports a duplicate-key error, which is how a lost race to
// create the same user or organization surfaces.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// googleAccountUser returns the user a Google subject is already linked to.
func googleAccountUser(ctx context.Context, q querier, subject string) (string, bool, error) {
	var userID string
	err := q.QueryRow(ctx, `SELECT user_id FROM account WHERE provider_id = 'google' AND account_id = $1 LIMIT 1`, subject).Scan(&userID)
	return found(userID, err, "google account")
}

// insertOAuthAccount links a provider identity to a user. No provider tokens
// are stored: they are not needed to sign someone in again.
func insertOAuthAccount(ctx context.Context, q querier, id, userID, providerID, accountID string, now time.Time) error {
	if _, err := q.Exec(ctx, `INSERT INTO account (id, account_id, provider_id, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)`, id, accountID, providerID, userID, now); err != nil {
		return fmt.Errorf("insert oauth account: %w", err)
	}
	return nil
}
