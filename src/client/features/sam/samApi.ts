import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

const projectInput = z.object({ projectId: z.string().min(1) });

const samSessionSchema = z.object({
  id: z.string(),
  title: z.string(),
  createdAt: z.string(),
  updatedAt: z.string(),
});

export type SamSession = z.infer<typeof samSessionSchema>;

export async function getSamAccessSetupStatus({
  data,
}: {
  data: { projectId: string };
}) {
  const { projectId } = projectInput.parse(data);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/sam/access-setup-status`,
    z.object({ enabled: z.boolean(), errorMessage: z.string().nullable() }),
    "POST",
    {},
  );
}

export async function listSamSessions({
  data,
}: {
  data: { projectId: string };
}) {
  const { projectId } = projectInput.parse(data);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/sam/sessions/list`,
    z.array(samSessionSchema),
    "POST",
    {},
  );
}

export async function createSamSession({
  data,
}: {
  data: { projectId: string };
}) {
  const { projectId } = projectInput.parse(data);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/sam/sessions/create`,
    z.object({ id: z.string() }),
    "POST",
    {},
  );
}

export async function archiveSamSession({
  data,
}: {
  data: { projectId: string; sessionId: string };
}) {
  const parsed = projectInput
    .extend({ sessionId: z.string().min(1) })
    .parse(data);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(parsed.projectId)}/sam/sessions/archive`,
    z.object({ ok: z.literal(true) }),
    "POST",
    { sessionId: parsed.sessionId },
  );
}
