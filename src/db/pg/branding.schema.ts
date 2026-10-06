import { pgTable, text } from "drizzle-orm/pg-core";
import { organization } from "./better-auth-schema";

// Postgres mirror of organization_branding. Column notes: see ../branding.schema.ts.
export const organizationBranding = pgTable("organization_branding", {
  organizationId: text("organization_id")
    .primaryKey()
    .references(() => organization.id, { onDelete: "cascade" }),
  brandName: text("brand_name").notNull(),
  accentColor: text("accent_color").notNull(),
  logoDataUrl: text("logo_data_url"),
  websiteUrl: text("website_url"),
  updatedAt: text("updated_at").notNull(),
});
