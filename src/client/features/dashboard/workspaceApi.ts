import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

export function getWorkspaceMergeStatus() {
  return apiRequest(
    "/api/v1/workspaces/legacy/status",
    z.object({ legacyWorkspaceCount: z.number().int().nonnegative() }),
    "POST",
    {},
  );
}

export function mergeLegacyWorkspaces() {
  return apiRequest(
    "/api/v1/workspaces/legacy/merge",
    z.object({ mergedWorkspaces: z.number().int().nonnegative() }),
    "POST",
    {},
  );
}
