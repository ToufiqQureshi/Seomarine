import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";
import type {
  CreateProjectInput,
  UpdateProjectInput,
  ArchiveProjectInput,
  RestoreProjectInput,
} from "@/types/schemas/projects";

const projectSchema = z.object({
  id: z.string(),
  name: z.string(),
  domain: z.string().nullable(),
  locationCode: z.number(),
  languageCode: z.string(),
  createdAt: z.string(),
});

type ProjectInput<T> = { data: T };

export async function getProjects() {
  return apiRequest(
    "/api/v1/projects/list",
    z.array(projectSchema),
    "POST",
    {},
  );
}

export async function createProject({
  data,
}: ProjectInput<CreateProjectInput>) {
  return apiRequest("/api/v1/projects/create", projectSchema, "POST", data);
}

export async function updateProject({
  data,
}: ProjectInput<UpdateProjectInput>) {
  const { projectId, ...input } = data;
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/update`,
    projectSchema,
    "POST",
    input,
  );
}

export async function archiveProject({
  data,
}: ProjectInput<ArchiveProjectInput>) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/archive`,
    z.object({ success: z.boolean() }),
    "POST",
    {},
  );
}

export async function getArchivedProjects() {
  return apiRequest(
    "/api/v1/projects/archived",
    z.array(projectSchema),
    "POST",
    {},
  );
}

export async function restoreProject({
  data,
}: ProjectInput<RestoreProjectInput>) {
  return apiRequest(
    "/api/v1/projects/restore",
    z.object({ success: z.boolean() }),
    "POST",
    data,
  );
}

export async function getProjectAccess({
  data,
}: ProjectInput<{ projectId: string }>) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/get`,
    projectSchema,
    "POST",
    {},
  );
}
