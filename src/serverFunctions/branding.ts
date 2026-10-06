import { createServerFn } from "@tanstack/react-start";
import { requireOrgPermission } from "@/server/auth/org-gate";
import { BrandingService } from "@/server/features/branding/services/BrandingService";
import { requireAuthenticatedContext } from "@/serverFunctions/middleware";
import { brandingInputSchema } from "@/types/schemas/branding";

// Organization-wide white-label branding for exported and shared reports.
// Anyone in the org can read it (the report viewer shows it); changing it is
// an organization setting, so it takes the same permission as renaming the org.

export const getBranding = createServerFn({ method: "GET" })
  .middleware(requireAuthenticatedContext)
  .handler(({ context }) =>
    BrandingService.getBranding(context.organizationId),
  );

export const saveBranding = createServerFn({ method: "POST" })
  .middleware(requireAuthenticatedContext)
  .validator(brandingInputSchema)
  .handler(async ({ data, context }) => {
    requireOrgPermission(context, { organization: ["update"] });
    await BrandingService.saveBranding(context.organizationId, data);
    return { ok: true as const };
  });

export const resetBranding = createServerFn({ method: "POST" })
  .middleware(requireAuthenticatedContext)
  .handler(async ({ context }) => {
    requireOrgPermission(context, { organization: ["update"] });
    await BrandingService.resetBranding(context.organizationId);
    return { ok: true as const };
  });
