import { apiRequest } from "@/client/lib/seomarineApi";
import type {
  ContextAuthor,
  ProjectContextUpdate,
} from "@/types/schemas/projectContext";
import { z } from "zod";

const contextAuthorSchema = z.enum(["user", "sam", "mcp"]);

const projectContextSchema = z.object({
  sections: z.array(
    z.object({
      key: z.string(),
      content: z.string(),
      updatedAt: z.string(),
      updatedBy: contextAuthorSchema,
    }),
  ),
  missingSections: z.array(z.string()),
  customSections: z.array(
    z.object({
      slug: z.string(),
      title: z.string().nullable(),
      content: z.string(),
      updatedAt: z.string(),
      updatedBy: contextAuthorSchema,
    }),
  ),
  competitors: z.array(
    z.object({
      id: z.string(),
      domain: z.string(),
      name: z.string().nullable(),
      notes: z.string().nullable(),
      updatedAt: z.string(),
      updatedBy: contextAuthorSchema,
    }),
  ),
  keyPages: z.array(
    z.object({
      id: z.string(),
      url: z.string(),
      role: z.enum(["hub", "spoke", "money", "other"]),
      topic: z.string().nullable(),
      notes: z.string().nullable(),
      updatedAt: z.string(),
      updatedBy: contextAuthorSchema,
    }),
  ),
  researchLog: z.array(
    z.object({
      id: z.string(),
      entryDate: z.string(),
      summary: z.string(),
      createdBy: contextAuthorSchema,
    }),
  ),
  reportTemplates: z.array(
    z.object({ name: z.string(), description: z.string() }),
  ),
});

export type ProjectContextData = z.infer<typeof projectContextSchema>;

type GetInput = { data: { projectId: string } };
type UpdateInput = {
  data: { projectId: string; updates: ProjectContextUpdate[] };
};

export async function getProjectContext({ data }: GetInput) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/context/get`,
    projectContextSchema,
    "POST",
    data,
  );
}

export async function updateProjectContext({ data }: UpdateInput) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/context/update`,
    projectContextSchema,
    "POST",
    data,
  );
}

export type { ContextAuthor };
