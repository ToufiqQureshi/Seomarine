-- +goose Up
CREATE TABLE go_audits (
    id text PRIMARY KEY,
    project_id text NOT NULL,
    started_by_user_id text NOT NULL,
    start_url text NOT NULL,
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed')),
    workflow_instance_id text,
    config text NOT NULL DEFAULT '{}',
    pages_crawled integer NOT NULL DEFAULT 0 CHECK (pages_crawled >= 0),
    pages_total integer NOT NULL DEFAULT 0 CHECK (pages_total >= 0),
    lighthouse_total integer NOT NULL DEFAULT 0 CHECK (lighthouse_total >= 0),
    lighthouse_completed integer NOT NULL DEFAULT 0 CHECK (lighthouse_completed >= 0),
    lighthouse_failed integer NOT NULL DEFAULT 0 CHECK (lighthouse_failed >= 0),
    current_phase text DEFAULT 'discovery',
    error_code text,
    error_detail text,
    failed_phase text,
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX go_audits_project_idx ON go_audits (project_id, started_at DESC);
CREATE INDEX go_audits_project_status_idx ON go_audits (project_id, status);

CREATE TABLE go_audit_pages (
    id text PRIMARY KEY,
    audit_id text NOT NULL REFERENCES go_audits (id) ON DELETE CASCADE,
    url text NOT NULL,
    status_code integer,
    redirect_url text,
    title text,
    meta_description text,
    canonical_url text,
    robots_meta text,
    x_robots_tag text,
    header_canonical_url text,
    og_title text,
    og_description text,
    og_image text,
    h1_count integer NOT NULL DEFAULT 0,
    h2_count integer NOT NULL DEFAULT 0,
    h3_count integer NOT NULL DEFAULT 0,
    h4_count integer NOT NULL DEFAULT 0,
    h5_count integer NOT NULL DEFAULT 0,
    h6_count integer NOT NULL DEFAULT 0,
    heading_order_json text,
    word_count integer NOT NULL DEFAULT 0,
    content_hash text,
    images_total integer NOT NULL DEFAULT 0,
    images_missing_alt integer NOT NULL DEFAULT 0,
    images_json text,
    internal_link_count integer NOT NULL DEFAULT 0,
    external_link_count integer NOT NULL DEFAULT 0,
    has_structured_data boolean NOT NULL DEFAULT false,
    hreflang_tags_json text,
    is_indexable boolean NOT NULL DEFAULT true,
    fetch_class text NOT NULL DEFAULT 'ok' CHECK (fetch_class IN ('ok', 'blocked', 'rate_limited', 'error')),
    crawl_depth integer,
    in_sitemap boolean NOT NULL DEFAULT false,
    response_time_ms integer
);

CREATE INDEX go_audit_pages_audit_idx ON go_audit_pages (audit_id);
CREATE UNIQUE INDEX go_audit_pages_audit_url_idx ON go_audit_pages (audit_id, url);

CREATE TABLE go_audit_issues (
    id text PRIMARY KEY,
    audit_id text NOT NULL REFERENCES go_audits (id) ON DELETE CASCADE,
    page_id text REFERENCES go_audit_pages (id) ON DELETE CASCADE,
    page_url text NOT NULL,
    issue_type text NOT NULL,
    severity text NOT NULL DEFAULT 'info' CHECK (severity IN ('critical', 'warning', 'info')),
    details_json text
);

CREATE INDEX go_audit_issues_audit_type_idx ON go_audit_issues (audit_id, issue_type);
CREATE INDEX go_audit_issues_page_id_idx ON go_audit_issues (page_id);

CREATE TABLE go_audit_lighthouse_results (
    id text PRIMARY KEY,
    audit_id text NOT NULL REFERENCES go_audits (id) ON DELETE CASCADE,
    page_id text NOT NULL REFERENCES go_audit_pages (id) ON DELETE CASCADE,
    url text NOT NULL,
    strategy text NOT NULL CHECK (strategy IN ('mobile', 'desktop')),
    performance_score integer,
    accessibility_score integer,
    best_practices_score integer,
    seo_score integer,
    lcp_ms double precision,
    cls double precision,
    inp_ms double precision,
    ttfb_ms double precision,
    error_message text,
    payload_json text,
    payload_size_bytes integer
);

CREATE INDEX go_audit_lighthouse_results_audit_idx ON go_audit_lighthouse_results (audit_id);
CREATE INDEX go_audit_lighthouse_results_page_idx ON go_audit_lighthouse_results (page_id);

-- +goose Down
DROP TABLE go_audit_lighthouse_results;
DROP TABLE go_audit_issues;
DROP TABLE go_audit_pages;
DROP TABLE go_audits;
