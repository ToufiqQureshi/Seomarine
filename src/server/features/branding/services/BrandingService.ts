import { BrandingRepository } from "@/server/features/branding/repositories/BrandingRepository";
import type { Branding, BrandingInput } from "@/types/schemas/branding";

// White-label report branding. Authorization is the caller's job: the server
// functions gate edits on the org role, and the report routes have already
// authorized the report whose organization they pass in.

function getBranding(organizationId: string): Promise<Branding | null> {
  return BrandingRepository.getBranding(organizationId);
}

function saveBranding(
  organizationId: string,
  input: BrandingInput,
): Promise<void> {
  return BrandingRepository.upsertBranding(organizationId, input);
}

/** Back to the default product branding. */
function resetBranding(organizationId: string): Promise<void> {
  return BrandingRepository.deleteBranding(organizationId);
}

export const BrandingService = {
  getBranding,
  saveBranding,
  resetBranding,
} as const;
