import { useQuery } from "@tanstack/react-query";
import { prewarmSerpLocations } from "@/client/features/keywords/keywordsApi";

/**
 * Warm the server-side location cache before the first keystroke, so the
 * country list is hot when the user searches. Best-effort: a failed warm just
 * means the first search is slower, so no retries, and staleTime keeps one
 * warm per country per session.
 */
export function usePrewarmSerpLocations(countryCode: string, enabled: boolean) {
  useQuery({
    queryKey: ["serp-locations-prewarm", countryCode],
    queryFn: () => prewarmSerpLocations({ countryCode }),
    enabled,
    staleTime: Infinity,
    retry: false,
  });
}
