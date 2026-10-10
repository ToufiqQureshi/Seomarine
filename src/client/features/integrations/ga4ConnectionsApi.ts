import { z } from "zod";
import { apiRequest } from "@/client/lib/seomarineApi";

const nullableText = z.string().nullable();

export const ga4ConnectionSchema = z.object({
  connected: z.boolean(),
  canManage: z.boolean(),
  currentUserHasGrant: z.boolean(),
  googleOAuthConfigured: z.boolean(),
  propertyId: nullableText,
  propertyDisplayName: nullableText,
  propertyTimeZone: nullableText,
  propertyCurrencyCode: nullableText,
  connectedByEmail: nullableText,
  connectedAt: nullableText,
});

const propertySchema = z.object({
  propertyId: z.string().regex(/^properties\/\d+$/),
  displayName: z.string(),
  accountDisplayName: z.string(),
  isSelected: z.boolean(),
});

const propertyAccountSchema = z.object({
  accountId: z.string(),
  email: nullableText,
  requiresReconnect: z.boolean(),
  propertiesUnavailable: z.boolean(),
  properties: z.array(propertySchema),
});

const propertyListSchema = z.object({
  accounts: z.array(propertyAccountSchema),
});

const savedConnectionSchema = z.object({
  connected: z.literal(true),
  propertyId: z.string(),
  propertyDisplayName: z.string(),
  propertyTimeZone: z.string(),
  propertyCurrencyCode: z.string(),
  connectedByEmail: nullableText,
  connectedAt: z.string(),
});

const disconnectedSchema = z.object({ connected: z.literal(false) });

function projectPath(projectId: string) {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/ga4`;
}

export function getGa4Connection(projectId: string) {
  return apiRequest(
    `${projectPath(projectId)}/connection/status`,
    ga4ConnectionSchema,
    "POST",
    {},
  );
}

export function listGa4Properties(projectId: string) {
  return apiRequest(
    `${projectPath(projectId)}/properties/list`,
    propertyListSchema,
    "POST",
    {},
  );
}

export function setGa4Property(
  projectId: string,
  selection: { accountId: string; propertyId: string },
) {
  return apiRequest(
    `${projectPath(projectId)}/connection/set`,
    savedConnectionSchema,
    "POST",
    selection,
  );
}

export function disconnectGa4(projectId: string) {
  return apiRequest(
    `${projectPath(projectId)}/connection/disconnect`,
    disconnectedSchema,
    "POST",
    {},
  );
}
