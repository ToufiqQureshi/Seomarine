import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

const membershipSchema = z.object({
  organizationId: z.string(),
  organizationName: z.string(),
  role: z.string(),
});

const organizationContextSchema = z.object({
  organizationId: z.string(),
  organizationName: z.string(),
  role: z.string(),
  organizations: z.array(membershipSchema),
});

export async function getOrganizationContext() {
  return apiRequest(
    "/api/v1/organization/context",
    organizationContextSchema,
    "GET",
  );
}

export async function switchOrganization({
  data,
}: {
  data: { organizationId: string };
}) {
  return apiRequest(
    "/api/v1/organization/switch",
    z.object({ organizationId: z.string() }),
    "POST",
    data,
  );
}
