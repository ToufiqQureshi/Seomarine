import { z } from "zod";
import { apiRequest } from "@/client/lib/uplumoApi";

const point = z.object({
  row: z.number(),
  col: z.number(),
  latitude: z.number(),
  longitude: z.number(),
  rank: z.number().nullable(),
  resultsCount: z.number(),
  error: z.boolean().optional(),
});
const gridResult = z.object({
  grid: z.array(point),
  summary: z.object({
    pointsSearched: z.number(),
    pointsFound: z.number(),
    averageRank: z.number().nullable(),
    top3Count: z.number(),
    top10Count: z.number(),
    failedPoints: z.number(),
  }),
  estimatedRequests: z.number(),
});
export type LocalGridResult = z.infer<typeof gridResult>;
type GridInput = {
  projectId: string;
  keyword: string;
  target: { cid?: string; placeId?: string; name?: string };
  center: { latitude: number; longitude: number };
  gridSize: 3 | 5;
  spacingKm: number;
  device: "mobile" | "desktop";
  languageCode: string;
};
export async function runLocalGrid({ projectId, ...body }: GridInput) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/local-seo/rank-grid`,
    gridResult,
    "POST",
    body,
  );
}
export async function findLocalBusinesses(
  projectId: string,
  body: {
    query: string;
    latitude: number;
    longitude: number;
    radiusKm: number;
  },
) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/local-seo/businesses`,
    z.object({ businesses: z.array(z.record(z.string(), z.unknown())) }),
    "POST",
    body,
  );
}
export async function getLocalBusinessProfile(
  projectId: string,
  body: {
    businessName?: string;
    cid?: string;
    placeId?: string;
    latitude: number;
    longitude: number;
    radiusKm: number;
    languageCode: string;
  },
) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/local-seo/profile`,
    z.object({ profile: z.record(z.string(), z.unknown()).nullable() }),
    "POST",
    body,
  );
}
