import { apiRequest } from "@/client/lib/seomarineApi";
import {
  brandLookupResultSchema,
  promptExplorerResultSchema,
  type BrandLookupInput,
  type BrandLookupResult,
  type PromptExplorerInput,
  type PromptExplorerResult,
} from "@/types/schemas/ai-search";

function projectPath(projectId: string, endpoint: string) {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/ai-search/${endpoint}`;
}

/** Brand Lookup: how AI platforms mention a brand or domain. */
export function lookupBrand({
  projectId,
  ...body
}: BrandLookupInput): Promise<BrandLookupResult> {
  return apiRequest(
    projectPath(projectId, "brand-lookup"),
    brandLookupResultSchema,
    "POST",
    body,
  );
}

/** Prompt Explorer: one prompt asked of up to four models. */
export function explorePrompt({
  projectId,
  ...body
}: PromptExplorerInput): Promise<PromptExplorerResult> {
  return apiRequest(
    projectPath(projectId, "prompt-explorer"),
    promptExplorerResultSchema,
    "POST",
    body,
  );
}
