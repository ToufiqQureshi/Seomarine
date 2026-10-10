# Auth Security & Session Management Design Note

## Overview

This design note outlines security, session compatibility, cryptographic practices, and rate limiting for the Go authentication subsystem in Seomarine.

## Security Practices

### 1. Better-Auth Session & Cipher Format Compatibility

- **Session Identification**: Go auth handlers validate session cookies created by better-auth (`better-auth.session_token` or `__Secure-better-auth.session_token`).
- **Session Table**: Active sessions are read from / written to `user_sessions` or `sessions` with pgx using strict parameterized SQL queries (`$1`, `$2`).
- **Token / Secret Ciphers**: Any token hash or cipher stored in Postgres uses compatible hashing (SHA-256 for tokens, Argon2id/Bcrypt for password hashes).

### 2. Constant-Time Comparisons

- Sensitive string/token comparisons (e.g. password verification tokens, API keys, session tokens, CSRF tokens) **must** use `subtle.ConstantTimeCompare` from `crypto/subtle` to eliminate timing side-channel attacks.

### 3. Rate Limiting via Redis

- All auth routes (`/api/v1/auth/login`, `/api/v1/auth/signup`, `/api/v1/auth/password-reset`, `/api/v1/auth/oauth/*`) enforce Redis-backed IP and user sliding-window rate limits via `backend/internal/kv`.
- Default limits:
  - Login attempts: max 5 requests per minute per IP.
  - Password reset requests: max 3 requests per hour per email/IP.
  - Signup attempts: max 3 requests per hour per IP.

### 4. Constant-Time Password Hash Verification

- Standard password hashing uses Argon2id or bcrypt. When an email is not found, password verification logic simulates equal execution time to prevent email enumeration.

### 5. Needs Security Review

- Note for PR body: **"needs security review"** tag added for all session, token, and password reset handling.
