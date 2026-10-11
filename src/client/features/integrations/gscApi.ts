import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

const projectInput = { projectId: z.string().min(1) };
type ProjectInput = { data: { projectId: string } };

const connectionSchema = z.object({
  connected: z.boolean(),
  canManage: z.boolean(),
  currentUserHasGrant: z.boolean(),
  googleOAuthConfigured: z.boolean(),
  siteUrl: z.string().nullable(),
  connectedByEmail: z.string().nullable(),
  connectedAt: z.string().nullable(),
});

const accountSchema = z.object({
  accountId: z.string(),
  email: z.string().nullable(),
  requiresReconnect: z.boolean(),
  propertiesUnavailable: z.boolean(),
  sites: z.array(
    z.object({
      siteUrl: z.string(),
      permissionLevel: z.string(),
      selectable: z.boolean(),
      isSelected: z.boolean(),
    }),
  ),
});

export async function getGscGrantStatus() {
  return apiRequest(
    "/api/v1/gsc/grant/status",
    z.object({ connected: z.boolean() }),
    "POST",
    {},
  );
}

export async function getGscConnection({ data }: ProjectInput) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/gsc/connection/status`,
    connectionSchema,
    "POST",
    {},
  );
}

export async function listGscSites({ data }: ProjectInput) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/gsc/sites/list`,
    z.object({ accounts: z.array(accountSchema) }),
    "POST",
    {},
  );
}

export async function setGscSite({
  data,
}: {
  data: { projectId: string; accountId: string; siteUrl: string };
}) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/gsc/connection/set`,
    z.object({
      connected: z.literal(true),
      siteUrl: z.string(),
      connectedByEmail: z.string().nullable(),
      connectedAt: z.string(),
    }),
    "POST",
    { accountId: data.accountId, siteUrl: data.siteUrl },
  );
}

export async function disconnectGsc({ data }: ProjectInput) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/gsc/connection/disconnect`,
    z.object({ connected: z.literal(false) }),
    "POST",
    {},
  );
}
