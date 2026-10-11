-- +goose Up
-- Drizzle PostgreSQL schema baseline. Generated from drizzle/pg in lexical order; preserve this snapshot as the legacy schema cutover boundary.

-- Source: 0000_fixed_nico_minoru.sql
CREATE TABLE "audit_lighthouse_results" (
	"id" text PRIMARY KEY NOT NULL,
	"audit_id" text NOT NULL,
	"page_id" text NOT NULL,
	"strategy" text NOT NULL,
	"performance_score" integer,
	"accessibility_score" integer,
	"best_practices_score" integer,
	"seo_score" integer,
	"lcp_ms" real,
	"cls" real,
	"inp_ms" real,
	"ttfb_ms" real,
	"error_message" text,
	"r2_key" text,
	"payload_size_bytes" integer
);
--> statement-breakpoint
CREATE TABLE "audit_pages" (
	"id" text PRIMARY KEY NOT NULL,
	"audit_id" text NOT NULL,
	"url" text NOT NULL,
	"status_code" integer,
	"redirect_url" text,
	"title" text,
	"meta_description" text,
	"canonical_url" text,
	"robots_meta" text,
	"og_title" text,
	"og_description" text,
	"og_image" text,
	"h1_count" integer DEFAULT 0 NOT NULL,
	"h2_count" integer DEFAULT 0 NOT NULL,
	"h3_count" integer DEFAULT 0 NOT NULL,
	"h4_count" integer DEFAULT 0 NOT NULL,
	"h5_count" integer DEFAULT 0 NOT NULL,
	"h6_count" integer DEFAULT 0 NOT NULL,
	"heading_order_json" text,
	"word_count" integer DEFAULT 0 NOT NULL,
	"images_total" integer DEFAULT 0 NOT NULL,
	"images_missing_alt" integer DEFAULT 0 NOT NULL,
	"images_json" text,
	"internal_link_count" integer DEFAULT 0 NOT NULL,
	"external_link_count" integer DEFAULT 0 NOT NULL,
	"has_structured_data" boolean DEFAULT false NOT NULL,
	"hreflang_tags_json" text,
	"is_indexable" boolean DEFAULT true NOT NULL,
	"response_time_ms" integer
);
--> statement-breakpoint
CREATE TABLE "audits" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"started_by_user_id" text NOT NULL,
	"start_url" text NOT NULL,
	"status" text DEFAULT 'running' NOT NULL,
	"workflow_instance_id" text,
	"config" text DEFAULT '{}' NOT NULL,
	"pages_crawled" integer DEFAULT 0 NOT NULL,
	"pages_total" integer DEFAULT 0 NOT NULL,
	"lighthouse_total" integer DEFAULT 0 NOT NULL,
	"lighthouse_completed" integer DEFAULT 0 NOT NULL,
	"lighthouse_failed" integer DEFAULT 0 NOT NULL,
	"current_phase" text DEFAULT 'discovery',
	"started_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"completed_at" text
);
--> statement-breakpoint
CREATE TABLE "keyword_metrics" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"keyword" text NOT NULL,
	"location_code" integer NOT NULL,
	"language_code" text DEFAULT 'en' NOT NULL,
	"search_volume" integer,
	"cpc" real,
	"competition" real,
	"keyword_difficulty" integer,
	"intent" text,
	"monthly_searches" text,
	"fetched_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "projects" (
	"id" text PRIMARY KEY NOT NULL,
	"organization_id" text NOT NULL,
	"name" text NOT NULL,
	"domain" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "rank_check_runs" (
	"id" text PRIMARY KEY NOT NULL,
	"config_id" text NOT NULL,
	"project_id" text NOT NULL,
	"status" text DEFAULT 'pending' NOT NULL,
	"keywords_total" integer DEFAULT 0 NOT NULL,
	"keywords_checked" integer DEFAULT 0 NOT NULL,
	"is_subset_run" boolean DEFAULT false NOT NULL,
	"error_message" text,
	"started_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"completed_at" text
);
--> statement-breakpoint
CREATE TABLE "rank_snapshots" (
	"id" serial PRIMARY KEY NOT NULL,
	"run_id" text NOT NULL,
	"tracking_keyword_id" text NOT NULL,
	"keyword" text NOT NULL,
	"device" text NOT NULL,
	"position" integer,
	"url" text,
	"serp_features" text,
	"checked_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "rank_tracking_configs" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"domain" text NOT NULL,
	"location_code" integer DEFAULT 2840 NOT NULL,
	"language_code" text DEFAULT 'en' NOT NULL,
	"devices" text DEFAULT 'both' NOT NULL,
	"serp_depth" integer NOT NULL,
	"schedule_interval" text DEFAULT 'weekly' NOT NULL,
	"is_active" boolean DEFAULT true NOT NULL,
	"last_checked_at" text,
	"next_check_at" text,
	"last_skip_reason" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "rank_tracking_keywords" (
	"id" text PRIMARY KEY NOT NULL,
	"config_id" text NOT NULL,
	"keyword" text NOT NULL,
	"search_volume" integer,
	"keyword_difficulty" integer,
	"cpc" real,
	"metrics_fetched_at" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "saved_keyword_tag_assignments" (
	"saved_keyword_id" text NOT NULL,
	"tag_id" text NOT NULL,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "saved_keyword_tags" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"name" text NOT NULL,
	"normalized_name" text NOT NULL,
	"color" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "saved_keywords" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"keyword" text NOT NULL,
	"location_code" integer DEFAULT 2840 NOT NULL,
	"language_code" text DEFAULT 'en' NOT NULL,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "user_onboarding_answers" (
	"user_id" text PRIMARY KEY NOT NULL,
	"organization_id" text NOT NULL,
	"interested_features" text DEFAULT '[]' NOT NULL,
	"work_for" text,
	"client_website_count" text,
	"found_via" text,
	"mcp_setup_intent" text,
	"completed_at" text,
	"gsc_nudge_dismissed_at" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "account" (
	"id" text PRIMARY KEY NOT NULL,
	"account_id" text NOT NULL,
	"provider_id" text NOT NULL,
	"user_id" text NOT NULL,
	"access_token" text,
	"refresh_token" text,
	"id_token" text,
	"access_token_expires_at" timestamp with time zone,
	"refresh_token_expires_at" timestamp with time zone,
	"scope" text,
	"password" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone NOT NULL
);
--> statement-breakpoint
CREATE TABLE "invitation" (
	"id" text PRIMARY KEY NOT NULL,
	"organization_id" text NOT NULL,
	"email" text NOT NULL,
	"role" text,
	"status" text DEFAULT 'pending' NOT NULL,
	"expires_at" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"inviter_id" text NOT NULL
);
--> statement-breakpoint
CREATE TABLE "member" (
	"id" text PRIMARY KEY NOT NULL,
	"organization_id" text NOT NULL,
	"user_id" text NOT NULL,
	"role" text DEFAULT 'member' NOT NULL,
	"created_at" timestamp with time zone NOT NULL
);
--> statement-breakpoint
CREATE TABLE "organization" (
	"id" text PRIMARY KEY NOT NULL,
	"name" text NOT NULL,
	"slug" text NOT NULL,
	"logo" text,
	"created_at" timestamp with time zone NOT NULL,
	"metadata" text,
	CONSTRAINT "organization_slug_unique" UNIQUE("slug")
);
--> statement-breakpoint
CREATE TABLE "session" (
	"id" text PRIMARY KEY NOT NULL,
	"expires_at" timestamp with time zone NOT NULL,
	"token" text NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	"ip_address" text,
	"user_agent" text,
	"user_id" text NOT NULL,
	"active_organization_id" text,
	CONSTRAINT "session_token_unique" UNIQUE("token")
);
--> statement-breakpoint
CREATE TABLE "user" (
	"id" text PRIMARY KEY NOT NULL,
	"name" text NOT NULL,
	"email" text NOT NULL,
	"email_verified" boolean DEFAULT false NOT NULL,
	"image" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"analytics_opted_out" boolean,
	CONSTRAINT "user_email_unique" UNIQUE("email")
);
--> statement-breakpoint
CREATE TABLE "verification" (
	"id" text PRIMARY KEY NOT NULL,
	"identifier" text NOT NULL,
	"value" text NOT NULL,
	"expires_at" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "gsc_connections" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"organization_id" text NOT NULL,
	"site_url" text NOT NULL,
	"connected_by_user_id" text NOT NULL,
	"connected_account_email" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "reddit_attributions" (
	"id" text PRIMARY KEY NOT NULL,
	"user_id" text NOT NULL,
	"organization_id" text NOT NULL,
	"click_id" text,
	"uuid" text,
	"landing_page" text,
	"referrer" text,
	"utm_source" text,
	"utm_medium" text,
	"utm_campaign" text,
	"utm_term" text,
	"utm_content" text,
	"signup_sent_at" text,
	"purchase_sent_at" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
ALTER TABLE "audit_lighthouse_results" ADD CONSTRAINT "audit_lighthouse_results_audit_id_audits_id_fk" FOREIGN KEY ("audit_id") REFERENCES "public"."audits"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_lighthouse_results" ADD CONSTRAINT "audit_lighthouse_results_page_id_audit_pages_id_fk" FOREIGN KEY ("page_id") REFERENCES "public"."audit_pages"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_pages" ADD CONSTRAINT "audit_pages_audit_id_audits_id_fk" FOREIGN KEY ("audit_id") REFERENCES "public"."audits"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audits" ADD CONSTRAINT "audits_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "keyword_metrics" ADD CONSTRAINT "keyword_metrics_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "projects" ADD CONSTRAINT "projects_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "rank_check_runs" ADD CONSTRAINT "rank_check_runs_config_id_rank_tracking_configs_id_fk" FOREIGN KEY ("config_id") REFERENCES "public"."rank_tracking_configs"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "rank_check_runs" ADD CONSTRAINT "rank_check_runs_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "rank_snapshots" ADD CONSTRAINT "rank_snapshots_run_id_rank_check_runs_id_fk" FOREIGN KEY ("run_id") REFERENCES "public"."rank_check_runs"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "rank_tracking_configs" ADD CONSTRAINT "rank_tracking_configs_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "rank_tracking_keywords" ADD CONSTRAINT "rank_tracking_keywords_config_id_rank_tracking_configs_id_fk" FOREIGN KEY ("config_id") REFERENCES "public"."rank_tracking_configs"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "saved_keyword_tag_assignments" ADD CONSTRAINT "saved_keyword_tag_assignments_saved_keyword_id_saved_keywords_id_fk" FOREIGN KEY ("saved_keyword_id") REFERENCES "public"."saved_keywords"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "saved_keyword_tag_assignments" ADD CONSTRAINT "saved_keyword_tag_assignments_tag_id_saved_keyword_tags_id_fk" FOREIGN KEY ("tag_id") REFERENCES "public"."saved_keyword_tags"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "saved_keyword_tags" ADD CONSTRAINT "saved_keyword_tags_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "saved_keywords" ADD CONSTRAINT "saved_keywords_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "user_onboarding_answers" ADD CONSTRAINT "user_onboarding_answers_user_id_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "user_onboarding_answers" ADD CONSTRAINT "user_onboarding_answers_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "account" ADD CONSTRAINT "account_user_id_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "invitation" ADD CONSTRAINT "invitation_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "invitation" ADD CONSTRAINT "invitation_inviter_id_user_id_fk" FOREIGN KEY ("inviter_id") REFERENCES "public"."user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "member" ADD CONSTRAINT "member_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "member" ADD CONSTRAINT "member_user_id_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "session" ADD CONSTRAINT "session_user_id_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "gsc_connections" ADD CONSTRAINT "gsc_connections_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "gsc_connections" ADD CONSTRAINT "gsc_connections_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "reddit_attributions" ADD CONSTRAINT "reddit_attributions_user_id_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "reddit_attributions" ADD CONSTRAINT "reddit_attributions_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "audit_lighthouse_results_audit_id_idx" ON "audit_lighthouse_results" USING btree ("audit_id");--> statement-breakpoint
CREATE INDEX "audit_pages_audit_id_idx" ON "audit_pages" USING btree ("audit_id");--> statement-breakpoint
CREATE INDEX "audits_project_id_idx" ON "audits" USING btree ("project_id");--> statement-breakpoint
CREATE INDEX "audits_started_by_user_id_idx" ON "audits" USING btree ("started_by_user_id");--> statement-breakpoint
CREATE UNIQUE INDEX "keyword_metrics_unique_project_keyword_location_language" ON "keyword_metrics" USING btree ("project_id","keyword","location_code","language_code");--> statement-breakpoint
CREATE INDEX "keyword_metrics_lookup_idx" ON "keyword_metrics" USING btree ("project_id","keyword","location_code","language_code","fetched_at");--> statement-breakpoint
CREATE UNIQUE INDEX "projects_one_default_per_organization_idx" ON "projects" USING btree ("organization_id") WHERE "projects"."name" = 'Default' AND "projects"."domain" IS NULL;--> statement-breakpoint
CREATE INDEX "rank_check_runs_config_idx" ON "rank_check_runs" USING btree ("config_id","started_at");--> statement-breakpoint
CREATE INDEX "rank_check_runs_project_idx" ON "rank_check_runs" USING btree ("project_id","started_at");--> statement-breakpoint
CREATE UNIQUE INDEX "rank_check_runs_one_active_per_config_idx" ON "rank_check_runs" USING btree ("config_id") WHERE "rank_check_runs"."status" IN ('pending', 'running');--> statement-breakpoint
CREATE INDEX "rank_snapshots_run_idx" ON "rank_snapshots" USING btree ("run_id");--> statement-breakpoint
CREATE INDEX "rank_snapshots_keyword_device_idx" ON "rank_snapshots" USING btree ("tracking_keyword_id","device","checked_at");--> statement-breakpoint
CREATE UNIQUE INDEX "rank_snapshots_run_keyword_device_idx" ON "rank_snapshots" USING btree ("run_id","tracking_keyword_id","device");--> statement-breakpoint
CREATE UNIQUE INDEX "rank_tracking_configs_project_domain_location_idx" ON "rank_tracking_configs" USING btree ("project_id","domain","location_code");--> statement-breakpoint
CREATE UNIQUE INDEX "rank_tracking_keywords_config_keyword_idx" ON "rank_tracking_keywords" USING btree ("config_id","keyword");--> statement-breakpoint
CREATE UNIQUE INDEX "saved_keyword_tag_assignments_unique_idx" ON "saved_keyword_tag_assignments" USING btree ("saved_keyword_id","tag_id");--> statement-breakpoint
CREATE INDEX "saved_keyword_tag_assignments_keyword_idx" ON "saved_keyword_tag_assignments" USING btree ("saved_keyword_id");--> statement-breakpoint
CREATE INDEX "saved_keyword_tag_assignments_tag_idx" ON "saved_keyword_tag_assignments" USING btree ("tag_id");--> statement-breakpoint
CREATE UNIQUE INDEX "saved_keyword_tags_project_normalized_name_idx" ON "saved_keyword_tags" USING btree ("project_id","normalized_name");--> statement-breakpoint
CREATE INDEX "saved_keyword_tags_project_name_idx" ON "saved_keyword_tags" USING btree ("project_id","name");--> statement-breakpoint
CREATE UNIQUE INDEX "saved_keywords_unique_project_keyword_location_language" ON "saved_keywords" USING btree ("project_id","keyword","location_code","language_code");--> statement-breakpoint
CREATE INDEX "saved_keywords_project_created_idx" ON "saved_keywords" USING btree ("project_id","created_at");--> statement-breakpoint
CREATE INDEX "user_onboarding_answers_organization_idx" ON "user_onboarding_answers" USING btree ("organization_id");--> statement-breakpoint
CREATE INDEX "account_userId_idx" ON "account" USING btree ("user_id");--> statement-breakpoint
CREATE INDEX "invitation_organizationId_idx" ON "invitation" USING btree ("organization_id");--> statement-breakpoint
CREATE INDEX "invitation_email_idx" ON "invitation" USING btree ("email");--> statement-breakpoint
CREATE INDEX "member_organizationId_idx" ON "member" USING btree ("organization_id");--> statement-breakpoint
CREATE INDEX "member_userId_idx" ON "member" USING btree ("user_id");--> statement-breakpoint
CREATE UNIQUE INDEX "organization_slug_uidx" ON "organization" USING btree ("slug");--> statement-breakpoint
CREATE INDEX "session_userId_idx" ON "session" USING btree ("user_id");--> statement-breakpoint
CREATE INDEX "verification_identifier_idx" ON "verification" USING btree ("identifier");--> statement-breakpoint
CREATE UNIQUE INDEX "gsc_connections_project_idx" ON "gsc_connections" USING btree ("project_id");--> statement-breakpoint
CREATE INDEX "gsc_connections_organization_idx" ON "gsc_connections" USING btree ("organization_id");--> statement-breakpoint
CREATE UNIQUE INDEX "reddit_attributions_user_idx" ON "reddit_attributions" USING btree ("user_id");--> statement-breakpoint
CREATE INDEX "reddit_attributions_organization_idx" ON "reddit_attributions" USING btree ("organization_id");

-- Source: 0001_striped_bulldozer.sql
CREATE TABLE "billing_customer_status" (
	"organization_id" text PRIMARY KEY NOT NULL,
	"is_paying" boolean DEFAULT false NOT NULL,
	"paid_plan_id" text,
	"paid_plan_status" text,
	"customer_json" text NOT NULL,
	"synced_at" text NOT NULL,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
DROP INDEX "projects_one_default_per_organization_idx";--> statement-breakpoint
ALTER TABLE "projects" ADD COLUMN "archived_at" text;--> statement-breakpoint
ALTER TABLE "billing_customer_status" ADD CONSTRAINT "billing_customer_status_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "projects_one_default_per_organization_idx" ON "projects" USING btree ("organization_id") WHERE "projects"."name" = 'Default' AND "projects"."domain" IS NULL AND "projects"."archived_at" IS NULL;

-- Source: 0002_clean_moira_mactaggert.sql
ALTER TABLE "projects" ADD COLUMN "location_code" integer DEFAULT 2840 NOT NULL;--> statement-breakpoint
ALTER TABLE "projects" ADD COLUMN "language_code" text DEFAULT 'en' NOT NULL;

-- Source: 0003_sturdy_may_parker.sql
CREATE INDEX "projects_organization_id_idx" ON "projects" USING btree ("organization_id");--> statement-breakpoint
CREATE INDEX "account_accountId_providerId_idx" ON "account" USING btree ("account_id","provider_id");--> statement-breakpoint
CREATE INDEX "verification_expiresAt_idx" ON "verification" USING btree ("expires_at");

-- Source: 0004_dashing_betty_ross.sql
DROP INDEX "rank_snapshots_run_idx";--> statement-breakpoint
DROP INDEX "saved_keyword_tag_assignments_keyword_idx";

-- Source: 0005_talented_wild_pack.sql
CREATE TABLE "sam_project_memory" (
	"project_id" text NOT NULL,
	"label" text NOT NULL,
	"content" text NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	CONSTRAINT "sam_project_memory_project_id_label_pk" PRIMARY KEY("project_id","label")
);
--> statement-breakpoint
CREATE TABLE "sam_sessions" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"user_id" text NOT NULL,
	"title" text DEFAULT 'New chat' NOT NULL,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"archived_at" text
);
--> statement-breakpoint
ALTER TABLE "sam_project_memory" ADD CONSTRAINT "sam_project_memory_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "sam_sessions" ADD CONSTRAINT "sam_sessions_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "sam_sessions" ADD CONSTRAINT "sam_sessions_user_id_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "sam_sessions_project_updated_idx" ON "sam_sessions" USING btree ("project_id","updated_at");

-- Source: 0006_location_name.sql
DROP INDEX "rank_tracking_configs_project_domain_location_idx";--> statement-breakpoint
ALTER TABLE "rank_tracking_configs" ADD COLUMN "location_name" text;--> statement-breakpoint
CREATE UNIQUE INDEX "rank_tracking_configs_national_idx" ON "rank_tracking_configs" USING btree ("project_id","domain","location_code") WHERE "rank_tracking_configs"."location_name" IS NULL;--> statement-breakpoint
CREATE UNIQUE INDEX "rank_tracking_configs_local_idx" ON "rank_tracking_configs" USING btree ("project_id","domain","location_code","location_name") WHERE "rank_tracking_configs"."location_name" IS NOT NULL;

-- Source: 0007_same_marvel_zombies.sql
CREATE TABLE "audit_issues" (
	"id" text PRIMARY KEY NOT NULL,
	"audit_id" text NOT NULL,
	"page_id" text,
	"page_url" text NOT NULL,
	"issue_type" text NOT NULL,
	"severity" text DEFAULT 'info' NOT NULL,
	"details_json" text
);
--> statement-breakpoint
CREATE TABLE "audit_links" (
	"id" text PRIMARY KEY NOT NULL,
	"audit_id" text NOT NULL,
	"source_page_id" text NOT NULL,
	"source_url" text NOT NULL,
	"target_url" text NOT NULL,
	"anchor" text,
	"is_internal" boolean DEFAULT true NOT NULL,
	"is_nofollow" boolean DEFAULT false NOT NULL
);
--> statement-breakpoint
ALTER TABLE "audit_pages" ADD COLUMN "x_robots_tag" text;--> statement-breakpoint
ALTER TABLE "audit_pages" ADD COLUMN "header_canonical_url" text;--> statement-breakpoint
ALTER TABLE "audit_pages" ADD COLUMN "crawl_depth" integer;--> statement-breakpoint
ALTER TABLE "audit_pages" ADD COLUMN "in_sitemap" boolean DEFAULT false NOT NULL;--> statement-breakpoint
ALTER TABLE "audit_pages" ADD COLUMN "content_hash" text;--> statement-breakpoint
ALTER TABLE "audit_pages" ADD COLUMN "fetch_class" text DEFAULT 'ok' NOT NULL;--> statement-breakpoint
ALTER TABLE "audit_issues" ADD CONSTRAINT "audit_issues_audit_id_audits_id_fk" FOREIGN KEY ("audit_id") REFERENCES "public"."audits"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_issues" ADD CONSTRAINT "audit_issues_page_id_audit_pages_id_fk" FOREIGN KEY ("page_id") REFERENCES "public"."audit_pages"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_links" ADD CONSTRAINT "audit_links_audit_id_audits_id_fk" FOREIGN KEY ("audit_id") REFERENCES "public"."audits"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "audit_links" ADD CONSTRAINT "audit_links_source_page_id_audit_pages_id_fk" FOREIGN KEY ("source_page_id") REFERENCES "public"."audit_pages"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "audit_issues_audit_id_idx" ON "audit_issues" USING btree ("audit_id");--> statement-breakpoint
CREATE INDEX "audit_issues_audit_type_idx" ON "audit_issues" USING btree ("audit_id","issue_type");--> statement-breakpoint
CREATE INDEX "audit_links_audit_id_idx" ON "audit_links" USING btree ("audit_id");--> statement-breakpoint
CREATE INDEX "audit_links_audit_target_idx" ON "audit_links" USING btree ("audit_id","target_url");--> statement-breakpoint
CREATE INDEX "audit_pages_audit_url_idx" ON "audit_pages" USING btree ("audit_id","url");

-- Source: 0008_yummy_annihilus.sql
ALTER TABLE "gsc_connections" ADD COLUMN "gsc_account_id" text;
--> statement-breakpoint
UPDATE gsc_connections SET gsc_account_id = (
  SELECT a.account_id FROM account a
  WHERE a.user_id = gsc_connections.connected_by_user_id
    AND a.provider_id = 'google-search-console'
)
WHERE gsc_account_id IS NULL
  AND (
    SELECT count(*) FROM account a2
    WHERE a2.user_id = gsc_connections.connected_by_user_id
      AND a2.provider_id = 'google-search-console'
  ) = 1;

-- Source: 0009_supreme_captain_stacy.sql
DROP INDEX "audit_issues_audit_id_idx";--> statement-breakpoint
DROP INDEX "audit_links_audit_id_idx";--> statement-breakpoint
DROP INDEX "audit_pages_audit_id_idx";--> statement-breakpoint
CREATE INDEX "rank_tracking_configs_project_active_created_idx" ON "rank_tracking_configs" USING btree ("project_id","is_active","created_at");

-- Source: 0010_overrated_amazoness.sql
CREATE INDEX "audit_issues_page_id_idx" ON "audit_issues" USING btree ("page_id");--> statement-breakpoint
CREATE INDEX "audit_lighthouse_results_page_id_idx" ON "audit_lighthouse_results" USING btree ("page_id");--> statement-breakpoint
CREATE INDEX "audit_links_source_page_id_idx" ON "audit_links" USING btree ("source_page_id");

-- Source: 0011_friendly_morlun.sql
CREATE TABLE "telemetry_state" (
	"id" integer PRIMARY KEY DEFAULT 1 NOT NULL,
	"install_id" text NOT NULL,
	"last_heartbeat_at" timestamp with time zone,
	"last_version" text,
	"mcp_tool_call_count" integer DEFAULT 0 NOT NULL
);

-- Source: 0012_dashboard.sql
CREATE TABLE "backlink_snapshots" (
	"id" serial PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"domain" text NOT NULL,
	"rank" integer,
	"backlinks" integer,
	"referring_domains" integer,
	"broken_backlinks" integer,
	"new_backlinks" integer,
	"lost_backlinks" integer,
	"new_referring_domains" integer,
	"lost_referring_domains" integer,
	"captured_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "organization_activation_state" (
	"organization_id" text PRIMARY KEY NOT NULL,
	"first_mcp_authorized_at" text,
	"first_mcp_tool_call_at" text,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "project_activation_state" (
	"project_id" text PRIMARY KEY NOT NULL,
	"competitor_step_clicked_at" text,
	"mcp_card_dismissed_at" text,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
ALTER TABLE "backlink_snapshots" ADD CONSTRAINT "backlink_snapshots_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "organization_activation_state" ADD CONSTRAINT "organization_activation_state_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_activation_state" ADD CONSTRAINT "project_activation_state_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "backlink_snapshots_project_captured_idx" ON "backlink_snapshots" USING btree ("project_id","captured_at");

-- Source: 0013_sleepy_black_tarantula.sql
ALTER TABLE "telemetry_state" ADD COLUMN "installed_at" timestamp with time zone;

-- Source: 0014_solid_centennial.sql
ALTER TABLE "audits" ADD COLUMN "error_code" text;--> statement-breakpoint
ALTER TABLE "audits" ADD COLUMN "error_detail" text;--> statement-breakpoint
ALTER TABLE "audits" ADD COLUMN "failed_phase" text;

-- Source: 0015_sticky_dagger.sql
DROP TABLE "audit_links" CASCADE;

-- Source: 0016_panoramic_blob.sql
ALTER TABLE "backlink_snapshots" ALTER COLUMN "backlinks" SET DATA TYPE bigint;--> statement-breakpoint
ALTER TABLE "backlink_snapshots" ALTER COLUMN "referring_domains" SET DATA TYPE bigint;--> statement-breakpoint
ALTER TABLE "backlink_snapshots" ALTER COLUMN "broken_backlinks" SET DATA TYPE bigint;--> statement-breakpoint
ALTER TABLE "backlink_snapshots" ALTER COLUMN "new_backlinks" SET DATA TYPE bigint;--> statement-breakpoint
ALTER TABLE "backlink_snapshots" ALTER COLUMN "lost_backlinks" SET DATA TYPE bigint;--> statement-breakpoint
ALTER TABLE "backlink_snapshots" ALTER COLUMN "new_referring_domains" SET DATA TYPE bigint;--> statement-breakpoint
ALTER TABLE "backlink_snapshots" ALTER COLUMN "lost_referring_domains" SET DATA TYPE bigint;

-- Source: 0017_ga4_connections.sql
CREATE TABLE "ga4_connections" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"organization_id" text NOT NULL,
	"property_id" text NOT NULL,
	"property_display_name" text NOT NULL,
	"property_time_zone" text NOT NULL,
	"property_currency_code" text NOT NULL,
	"connected_by_user_id" text NOT NULL,
	"ga4_account_id" text NOT NULL,
	"connected_account_email" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
ALTER TABLE "project_activation_state" ADD COLUMN "ga4_card_dismissed_at" text;--> statement-breakpoint
ALTER TABLE "ga4_connections" ADD CONSTRAINT "ga4_connections_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "ga4_connections" ADD CONSTRAINT "ga4_connections_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "ga4_connections_project_idx" ON "ga4_connections" USING btree ("project_id");--> statement-breakpoint
CREATE INDEX "ga4_connections_organization_idx" ON "ga4_connections" USING btree ("organization_id");--> statement-breakpoint
CREATE INDEX "ga4_connections_connector_idx" ON "ga4_connections" USING btree ("connected_by_user_id","ga4_account_id");

-- Source: 0018_drop_reddit_attributions.sql
DROP TABLE "reddit_attributions" CASCADE;

-- Source: 0019_clammy_selene.sql
CREATE TABLE "apikey" (
	"id" text PRIMARY KEY NOT NULL,
	"config_id" text DEFAULT 'default' NOT NULL,
	"name" text,
	"start" text,
	"prefix" text,
	"key" text NOT NULL,
	"reference_id" text NOT NULL,
	"refill_interval" integer,
	"refill_amount" integer,
	"last_refill_at" timestamp with time zone,
	"enabled" boolean DEFAULT true,
	"rate_limit_enabled" boolean DEFAULT true,
	"rate_limit_time_window" integer DEFAULT 60000,
	"rate_limit_max" integer DEFAULT 120,
	"request_count" integer DEFAULT 0,
	"remaining" integer,
	"last_request" timestamp with time zone,
	"expires_at" timestamp with time zone,
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	"permissions" text,
	"metadata" text
);
--> statement-breakpoint
CREATE INDEX "apikey_configId_idx" ON "apikey" USING btree ("config_id");--> statement-breakpoint
CREATE INDEX "apikey_referenceId_idx" ON "apikey" USING btree ("reference_id");--> statement-breakpoint
CREATE INDEX "apikey_key_idx" ON "apikey" USING btree ("key");

-- Source: 0020_project_memory.sql
CREATE TABLE "project_competitors" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"domain" text NOT NULL,
	"name" text,
	"notes" text,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_by" text NOT NULL
);
--> statement-breakpoint
CREATE TABLE "project_context_sections" (
	"project_id" text NOT NULL,
	"key" text NOT NULL,
	"title" text,
	"content" text NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_by" text NOT NULL,
	CONSTRAINT "project_context_sections_project_id_key_pk" PRIMARY KEY("project_id","key")
);
--> statement-breakpoint
CREATE TABLE "project_key_pages" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"url" text NOT NULL,
	"role" text NOT NULL,
	"topic" text,
	"notes" text,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_by" text NOT NULL
);
--> statement-breakpoint
CREATE TABLE "project_research_log" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"entry_date" text NOT NULL,
	"summary" text NOT NULL,
	"created_by" text NOT NULL,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
ALTER TABLE "project_competitors" ADD CONSTRAINT "project_competitors_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_context_sections" ADD CONSTRAINT "project_context_sections_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_key_pages" ADD CONSTRAINT "project_key_pages_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "project_research_log" ADD CONSTRAINT "project_research_log_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "project_competitors_project_domain_idx" ON "project_competitors" USING btree ("project_id","domain");--> statement-breakpoint
CREATE UNIQUE INDEX "project_key_pages_project_url_idx" ON "project_key_pages" USING btree ("project_id","url");--> statement-breakpoint
CREATE INDEX "project_research_log_project_date_idx" ON "project_research_log" USING btree ("project_id","entry_date");
--> statement-breakpoint
DROP TABLE "sam_project_memory" CASCADE;

-- Source: 0021_user_last_active_organization.sql
ALTER TABLE "user" ADD COLUMN "last_active_organization_id" text;

-- Source: 0022_third_supernaut.sql
CREATE UNIQUE INDEX "member_organizationId_userId_uidx" ON "member" USING btree ("organization_id","user_id");

-- Source: 0023_clammy_screwball.sql
ALTER TABLE "rank_tracking_keywords" ADD COLUMN "match_case" boolean DEFAULT false NOT NULL;

-- Source: 0024_tense_echo.sql
CREATE TABLE "dashboard_step_dismissals" (
	"user_id" text NOT NULL,
	"project_id" text NOT NULL,
	"step" text NOT NULL,
	CONSTRAINT "dashboard_step_dismissals_user_id_project_id_step_pk" PRIMARY KEY("user_id","project_id","step")
);
--> statement-breakpoint
ALTER TABLE "dashboard_step_dismissals" ADD CONSTRAINT "dashboard_step_dismissals_user_id_user_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."user"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "dashboard_step_dismissals" ADD CONSTRAINT "dashboard_step_dismissals_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "dashboard_step_dismissals_project_idx" ON "dashboard_step_dismissals" USING btree ("project_id");

-- Source: 0025_reports.sql
CREATE TABLE "reports" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"title" text NOT NULL,
	"summary" text NOT NULL,
	"html" text NOT NULL,
	"skill" text,
	"template_id" text,
	"created_by" text NOT NULL,
	"created_by_user_id" text NOT NULL,
	"size_bytes" integer NOT NULL,
	"share_token" text,
	"shared_at" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
CREATE TABLE "report_templates" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"name" text NOT NULL,
	"description" text NOT NULL,
	"instructions" text NOT NULL,
	"created_by" text NOT NULL,
	"created_by_user_id" text NOT NULL,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL,
	"updated_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
ALTER TABLE "reports" ADD CONSTRAINT "reports_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "report_templates" ADD CONSTRAINT "report_templates_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "reports_project_updated_idx" ON "reports" USING btree ("project_id","updated_at","id","size_bytes");--> statement-breakpoint
CREATE UNIQUE INDEX "reports_share_token_idx" ON "reports" USING btree ("share_token");--> statement-breakpoint
CREATE UNIQUE INDEX "report_templates_project_name_idx" ON "report_templates" USING btree ("project_id","name");

-- Source: 0026_minor_dreadnoughts.sql
CREATE TABLE "crawler_credentials" (
	"id" text PRIMARY KEY NOT NULL,
	"project_id" text NOT NULL,
	"host" text NOT NULL,
	"provider" text NOT NULL,
	"signature_input" text NOT NULL,
	"signature" text NOT NULL,
	"expires_at" text,
	"created_by_user_id" text,
	"created_at" text DEFAULT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') NOT NULL
);
--> statement-breakpoint
ALTER TABLE "crawler_credentials" ADD CONSTRAINT "crawler_credentials_project_id_projects_id_fk" FOREIGN KEY ("project_id") REFERENCES "public"."projects"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "crawler_credentials_project_host_idx" ON "crawler_credentials" USING btree ("project_id","host");

-- Source: 0027_keyword_step.sql
ALTER TABLE "project_activation_state" ADD COLUMN "keyword_step_clicked_at" text;

-- Source: 0028_modern_nekra.sql
DELETE FROM "account" WHERE "id" IN (
  SELECT "id" FROM (
    SELECT "id", ROW_NUMBER() OVER (
      PARTITION BY "user_id", "provider_id", "account_id"
      ORDER BY ("refresh_token" IS NOT NULL) DESC, "updated_at" DESC, "id" DESC
    ) AS "row_number"
    FROM "account"
    WHERE "provider_id" IN ('google-search-console', 'google-analytics')
  ) AS "duplicate_grants"
  WHERE "row_number" > 1
);
--> statement-breakpoint
CREATE UNIQUE INDEX IF NOT EXISTS "account_google_grant_owner_idx" ON "account" USING btree ("user_id","provider_id","account_id") WHERE "account"."provider_id" in ('google-search-console', 'google-analytics');

-- Source: 0029_organization_branding.sql
CREATE TABLE "organization_branding" (
	"organization_id" text PRIMARY KEY NOT NULL,
	"brand_name" text NOT NULL,
	"accent_color" text NOT NULL,
	"logo_data_url" text,
	"website_url" text,
	"updated_at" text NOT NULL
);
--> statement-breakpoint
ALTER TABLE "organization_branding" ADD CONSTRAINT "organization_branding_organization_id_organization_id_fk" FOREIGN KEY ("organization_id") REFERENCES "public"."organization"("id") ON DELETE cascade ON UPDATE no action;


-- The original Go migration version 1 table is part of this baseline so existing Goose versions stay compatible.
CREATE TABLE go_analytics_sites (
    project_id text PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    site_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS go_analytics_sites CASCADE;

-- Remove the legacy application schema created by this baseline. Goose applies later Go migrations down first.

DROP TABLE IF EXISTS "organization_branding" CASCADE;
DROP TABLE IF EXISTS "crawler_credentials" CASCADE;
DROP TABLE IF EXISTS "report_templates" CASCADE;
DROP TABLE IF EXISTS "reports" CASCADE;
DROP TABLE IF EXISTS "dashboard_step_dismissals" CASCADE;
DROP TABLE IF EXISTS "project_research_log" CASCADE;
DROP TABLE IF EXISTS "project_key_pages" CASCADE;
DROP TABLE IF EXISTS "project_context_sections" CASCADE;
DROP TABLE IF EXISTS "project_competitors" CASCADE;
DROP TABLE IF EXISTS "apikey" CASCADE;
DROP TABLE IF EXISTS "ga4_connections" CASCADE;
DROP TABLE IF EXISTS "project_activation_state" CASCADE;
DROP TABLE IF EXISTS "organization_activation_state" CASCADE;
DROP TABLE IF EXISTS "backlink_snapshots" CASCADE;
DROP TABLE IF EXISTS "telemetry_state" CASCADE;
DROP TABLE IF EXISTS "audit_links" CASCADE;
DROP TABLE IF EXISTS "audit_issues" CASCADE;
DROP TABLE IF EXISTS "sam_sessions" CASCADE;
DROP TABLE IF EXISTS "sam_project_memory" CASCADE;
DROP TABLE IF EXISTS "billing_customer_status" CASCADE;
DROP TABLE IF EXISTS "reddit_attributions" CASCADE;
DROP TABLE IF EXISTS "gsc_connections" CASCADE;
DROP TABLE IF EXISTS "verification" CASCADE;
DROP TABLE IF EXISTS "user" CASCADE;
DROP TABLE IF EXISTS "session" CASCADE;
DROP TABLE IF EXISTS "organization" CASCADE;
DROP TABLE IF EXISTS "member" CASCADE;
DROP TABLE IF EXISTS "invitation" CASCADE;
DROP TABLE IF EXISTS "account" CASCADE;
DROP TABLE IF EXISTS "user_onboarding_answers" CASCADE;
DROP TABLE IF EXISTS "saved_keywords" CASCADE;
DROP TABLE IF EXISTS "saved_keyword_tags" CASCADE;
DROP TABLE IF EXISTS "saved_keyword_tag_assignments" CASCADE;
DROP TABLE IF EXISTS "rank_tracking_keywords" CASCADE;
DROP TABLE IF EXISTS "rank_tracking_configs" CASCADE;
DROP TABLE IF EXISTS "rank_snapshots" CASCADE;
DROP TABLE IF EXISTS "rank_check_runs" CASCADE;
DROP TABLE IF EXISTS "projects" CASCADE;
DROP TABLE IF EXISTS "keyword_metrics" CASCADE;
DROP TABLE IF EXISTS "audits" CASCADE;
DROP TABLE IF EXISTS "audit_pages" CASCADE;
DROP TABLE IF EXISTS "audit_lighthouse_results" CASCADE;
