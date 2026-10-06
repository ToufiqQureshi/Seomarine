import { sqliteTable, text } from "drizzle-orm/sqlite-core";
import { organization } from "./better-auth-schema";

// White-label branding for an organization's client-facing reports: the bar
// stamped onto exported PDFs and the public share page. One row per org; no
// row means the default product branding. The logo is a small data URL rather
// than an R2 object or a remote URL, because the report sandbox CSP only allows
// `img-src data:` and the service caps it at BRANDING_MAX_LOGO_CHARS.
export const organizationBranding = sqliteTable("organization_branding", {
  organizationId: text("organization_id")
    .primaryKey()
    .references(() => organization.id, { onDelete: "cascade" }),
  brandName: text("brand_name").notNull(),
  accentColor: text("accent_color").notNull(),
  logoDataUrl: text("logo_data_url"),
  websiteUrl: text("website_url"),
  updatedAt: text("updated_at").notNull(),
});
