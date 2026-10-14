import { z } from "zod";
import { apiRequest, shouldRetryApiError } from "@/client/lib/uplumoApi";

const connectionSchema = z.object({
  connected: z.boolean(),
  platform: z.literal("wordpress").optional(),
  siteUrl: z.string().optional(),
  siteName: z.string().optional(),
  username: z.string().optional(),
  connectedAt: z.iso.datetime({ offset: true }).optional(),
});
export type SiteConnection = z.infer<typeof connectionSchema>;

const publicationSchema = z.object({
  published: z.boolean(),
  siteUrl: z.string().optional(),
  url: z.string().optional(),
  status: z.string().optional(),
  publishedAt: z.iso.datetime({ offset: true }).optional(),
  updatedAt: z.iso.datetime({ offset: true }).optional(),
});
export type SitePublication = z.infer<typeof publicationSchema>;

const projectPath = (projectId: string) =>
  `/api/v1/projects/${encodeURIComponent(projectId)}`;

export const siteConnectionQueryOptions = (projectId: string) => ({
  queryKey: ["siteConnection", projectId],
  queryFn: () =>
    apiRequest(
      `${projectPath(projectId)}/site-connection/get`,
      connectionSchema,
      "POST",
      {},
    ),
  retry: shouldRetryApiError,
});

export const sitePublicationQueryOptions = (
  projectId: string,
  draftId: string,
) => ({
  queryKey: ["sitePublication", projectId, draftId],
  queryFn: () =>
    apiRequest(
      `${projectPath(projectId)}/content/drafts/${encodeURIComponent(draftId)}/publication/get`,
      publicationSchema,
      "POST",
      {},
    ),
  retry: shouldRetryApiError,
});

export function connectWordPress(
  projectId: string,
  input: { siteUrl: string; username: string; applicationPassword: string },
): Promise<SiteConnection> {
  return apiRequest(
    `${projectPath(projectId)}/site-connection/save`,
    connectionSchema,
    "POST",
    { platform: "wordpress", ...input },
  );
}

export function disconnectSite(projectId: string): Promise<SiteConnection> {
  return apiRequest(
    `${projectPath(projectId)}/site-connection/delete`,
    connectionSchema,
    "POST",
    {},
  );
}

export function publishDraft(
  projectId: string,
  draftId: string,
  status: "draft" | "publish",
): Promise<SitePublication> {
  return apiRequest(
    `${projectPath(projectId)}/content/drafts/${encodeURIComponent(draftId)}/publish`,
    publicationSchema,
    "POST",
    { status },
  );
}
