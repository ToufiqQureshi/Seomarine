import { eq } from "drizzle-orm";
import { db } from "@/db";
import { organizationBranding } from "@/db/schema";
import type { Branding, BrandingInput } from "@/types/schemas/branding";

// One row per organization. Callers pass the organization id from the
// authenticated context (or from the project a share token resolved to).

const columns = {
  brandName: organizationBranding.brandName,
  accentColor: organizationBranding.accentColor,
  logoDataUrl: organizationBranding.logoDataUrl,
  websiteUrl: organizationBranding.websiteUrl,
  updatedAt: organizationBranding.updatedAt,
};

async function getBranding(organizationId: string): Promise<Branding | null> {
  const [row] = await db
    .select(columns)
    .from(organizationBranding)
    .where(eq(organizationBranding.organizationId, organizationId))
    .limit(1);
  return row ?? null;
}

async function upsertBranding(
  organizationId: string,
  input: BrandingInput,
): Promise<void> {
  const values = { ...input, updatedAt: new Date().toISOString() };
  await db
    .insert(organizationBranding)
    .values({ organizationId, ...values })
    .onConflictDoUpdate({
      target: organizationBranding.organizationId,
      set: values,
    });
}

async function deleteBranding(organizationId: string): Promise<void> {
  await db
    .delete(organizationBranding)
    .where(eq(organizationBranding.organizationId, organizationId));
}

export const BrandingRepository = {
  getBranding,
  upsertBranding,
  deleteBranding,
} as const;
