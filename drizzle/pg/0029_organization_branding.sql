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