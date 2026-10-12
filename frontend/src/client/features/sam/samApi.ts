import type { UIMessage } from "ai";
import { apiRequest } from "@/client/lib/uplumoApi";
import { z } from "zod";

const projectInput = z.object({ projectId: z.string().min(1) });

const samSessionSchema = z.object({
  id: z.string(),
  title: z.string(),
  createdAt: z.string(),
  updatedAt: z.string(),
});

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

// Stored messages are AI SDK UIMessages: the server builds the parts from the
// same events it streams, so the chat view renders history and live replies
// the same way.
const samMessageSchema = z.object({
  id: z.string(),
  role: z.enum(["user", "assistant"]),
  parts: z.array(z.looseObject({ type: z.string() })),
});

export async function listSamMessages({
  data,
}: {
  data: { projectId: string; sessionId: string };
}): Promise<UIMessage[]> {
  const parsed = projectInput
    .extend({ sessionId: z.string().min(1) })
    .parse(data);
  const messages = await apiRequest(
    `/api/v1/projects/${encodeURIComponent(parsed.projectId)}/sam/sessions/messages`,
    z.array(samMessageSchema),
    "POST",
    { sessionId: parsed.sessionId },
  );
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- the parts are the server's own UIMessage parts; the schema checks their envelope, not every tool shape
  return messages as UIMessage[];
}

export async function rewindSamSession({
  data,
}: {
  data: { projectId: string; sessionId: string; messageId: string };
}) {
  const parsed = projectInput
    .extend({ sessionId: z.string().min(1), messageId: z.string().min(1) })
    .parse(data);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(parsed.projectId)}/sam/sessions/rewind`,
    z.object({ ok: z.literal(true) }),
    "POST",
    { sessionId: parsed.sessionId, messageId: parsed.messageId },
  );
}

export function samChatUrl(projectId: string): string {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/sam/sessions/chat`;
}
