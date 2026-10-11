import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

const projectPath = (projectId: string) =>
  `/api/v1/projects/${encodeURIComponent(projectId)}/backlinks/domain-ratings`;

const ratingsSchema = z.record(
  z.string(),
  z.number().min(0).max(100).nullable(),
);

export async function getAhrefsDomainRatings({
  data,
}: {
  data: { projectId: string; domains: string[] };
}) {
  return apiRequest(projectPath(data.projectId), ratingsSchema, "POST", {
    domains: data.domains,
  });
}
