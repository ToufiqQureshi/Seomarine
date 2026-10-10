-- +goose Up
-- MCP OAuth grants and client registrations. Owned by the Go server so the
-- legacy app's workers-oauth-provider KV is not a dependency.
CREATE TABLE go_mcp_oauth_clients (
    id text PRIMARY KEY,
    client_id text NOT NULL UNIQUE,
    client_secret text NOT NULL,
    redirect_uris text[] NOT NULL DEFAULT '{}',
    token_endpoint_auth_method text NOT NULL DEFAULT 'none',
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX go_mcp_oauth_clients_client_id_idx ON go_mcp_oauth_clients (client_id);

CREATE TABLE go_mcp_oauth_authorization_codes (
    code text PRIMARY KEY,
    client_id text NOT NULL REFERENCES go_mcp_oauth_clients (id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    scope text NOT NULL,
    user_id text NOT NULL,
    organization_id text NOT NULL,
    pkce_challenge text,
    pkce_method text,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX go_mcp_oauth_auth_codes_client_idx ON go_mcp_oauth_authorization_codes (client_id);
CREATE INDEX go_mcp_oauth_auth_codes_user_idx ON go_mcp_oauth_authorization_codes (user_id);

CREATE TABLE go_mcp_oauth_access_tokens (
    token text PRIMARY KEY,
    client_id text NOT NULL REFERENCES go_mcp_oauth_clients (id) ON DELETE CASCADE,
    scope text NOT NULL,
    user_id text NOT NULL,
    organization_id text NOT NULL,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX go_mcp_oauth_access_client_idx ON go_mcp_oauth_access_tokens (client_id);
CREATE INDEX go_mcp_oauth_access_user_idx ON go_mcp_oauth_access_tokens (user_id);

CREATE TABLE go_mcp_oauth_refresh_tokens (
    token text PRIMARY KEY,
    access_token text NOT NULL REFERENCES go_mcp_oauth_access_tokens (token) ON DELETE CASCADE,
    client_id text NOT NULL,
    scope text NOT NULL,
    user_id text NOT NULL,
    organization_id text NOT NULL,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX go_mcp_oauth_refresh_client_idx ON go_mcp_oauth_refresh_tokens (client_id);
CREATE INDEX go_mcp_oauth_refresh_user_idx ON go_mcp_oauth_refresh_tokens (user_id);

-- +goose Down
DROP TABLE go_mcp_oauth_refresh_tokens;
DROP TABLE go_mcp_oauth_access_tokens;
DROP TABLE go_mcp_oauth_authorization_codes;
DROP TABLE go_mcp_oauth_clients;