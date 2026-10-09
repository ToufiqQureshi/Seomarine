import { skipToken, useQuery } from "@tanstack/react-query";
import { getDomainOverview } from "@/client/features/domain/domainApi";
import { useDomainMarket } from "@/client/features/domain/domainMarket";
import type { ResearchScope } from "@/shared/researchScope";

type Input = {
  projectId: string;
  domain: string;
  scope: ResearchScope;
  locationCode: number | undefined;
};

export function useDomainOverviewQuery(input: Input) {
  const trimmedDomain = input.domain.trim();
  const market = useDomainMarket(input.projectId, input.locationCode);

  return useQuery({
    enabled: trimmedDomain !== "",
    queryKey: [
      "domain-overview",
      input.projectId,
      trimmedDomain,
      input.scope,
      input.locationCode,
    ],
    queryFn: market
      ? () =>
          getDomainOverview({
            data: {
              projectId: input.projectId,
              domain: trimmedDomain,
              scope: input.scope,
              ...market,
            },
          })
      : skipToken,
    staleTime: 5 * 60_000,
  });
}
