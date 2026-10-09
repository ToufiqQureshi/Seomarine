import { z } from "zod";
import { apiRequest } from "@/client/lib/seomarineApi";

const serpLocationSchema = z.object({
  locationCode: z.number(),
  locationName: z.string(),
  locationType: z.string(),
  displayLabel: z.string(),
});

const serpLocationsSchema = z.array(serpLocationSchema);
const prewarmSchema = z.object({ warmed: z.boolean() });

export type SerpLocationResult = z.infer<typeof serpLocationSchema>;

export function searchSerpLocations(input: {
  countryCode: string;
  query: string;
}) {
  return apiRequest(
    "/api/v1/serp-locations/search",
    serpLocationsSchema,
    "POST",
    input,
  );
}

export function prewarmSerpLocations(input: { countryCode: string }) {
  return apiRequest(
    "/api/v1/serp-locations/prewarm",
    prewarmSchema,
    "POST",
    input,
  );
}
