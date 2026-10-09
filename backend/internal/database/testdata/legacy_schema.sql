-- The legacy app's tables that the Go server reads, as the legacy Drizzle
-- schema (src/db/pg) creates them. Tests apply this to an empty database;
-- production already has them. The advisory lock serializes test packages
-- that run in parallel against the same database.
BEGIN;
SELECT pg_advisory_xact_lock(7340001);

CREATE TABLE IF NOT EXISTS "user" (
    id text PRIMARY KEY,
    name text NOT NULL,
    email text NOT NULL UNIQUE,
    email_verified boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS session (
    id text PRIMARY KEY,
    expires_at timestamptz NOT NULL,
    token text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    user_id text NOT NULL REFERENCES "user" (id) ON DELETE CASCADE,
    active_organization_id text
);

CREATE TABLE IF NOT EXISTS organization (
    id text PRIMARY KEY,
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS member (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organization (id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES "user" (id) ON DELETE CASCADE,
    role text NOT NULL DEFAULT 'member',
    created_at timestamptz NOT NULL,
    UNIQUE (organization_id, user_id)
);

CREATE TABLE IF NOT EXISTS organization_branding (
    organization_id text PRIMARY KEY REFERENCES organization (id) ON DELETE CASCADE,
    brand_name text NOT NULL,
    accent_color text NOT NULL,
    logo_data_url text,
    website_url text,
    updated_at text NOT NULL
);

-- Legacy timestamps in app tables are ISO-8601 text, not timestamptz.
CREATE TABLE IF NOT EXISTS projects (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organization (id) ON DELETE CASCADE,
    name text NOT NULL,
    domain text,
    location_code integer NOT NULL DEFAULT 2840,
    language_code text NOT NULL DEFAULT 'en',
    created_at text NOT NULL DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
    archived_at text
);

COMMIT;
