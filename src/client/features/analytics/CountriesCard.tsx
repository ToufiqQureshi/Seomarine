import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Globe2 } from "lucide-react";
import { CardShell } from "@/client/components/CardShell";
import { Button } from "@/client/components/ui/button";
import { Skeleton } from "@/client/components/ui/skeleton";
import {
  analyticsCountriesPage,
  analyticsSiteQueryOptions,
} from "./analyticsApi";

function flag(code: string) {
  return String.fromCodePoint(
    127397 + code.charCodeAt(0),
    127397 + code.charCodeAt(1),
  );
}

export function CountriesCard({
  projectId,
  range,
}: {
  projectId: string;
  range: { from: string; to: string };
}) {
  const site = useQuery(analyticsSiteQueryOptions(projectId));
  const countries = useInfiniteQuery({
    queryKey: ["goAnalyticsCountries", site.data?.id, range.from, range.to],
    queryFn: ({ pageParam }) => {
      if (!site.data) throw new Error("Analytics site is unavailable");
      return analyticsCountriesPage(site.data.id, range, pageParam);
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage, pages) =>
      lastPage.length === 10 ? pages.length + 1 : undefined,
    enabled: Boolean(site.data?.id),
    staleTime: 60_000,
  });
  const rows = countries.data?.pages.flat() ?? [];
  const isPending = site.isPending || countries.isPending;
  const error = site.error ?? countries.error;

  return (
    <CardShell
      title="Countries"
      icon={<Globe2 className="size-4 text-primary" />}
    >
      {isPending ? (
        <div className="space-y-3" aria-label="Loading countries" aria-busy>
          {Array.from({ length: 5 }, (_, index) => (
            <Skeleton key={index} className="h-8 w-full" />
          ))}
        </div>
      ) : error ? (
        <div className="space-y-3 text-sm">
          <p className="text-destructive">Countries could not be loaded.</p>
          <Button
            size="sm"
            variant="outline"
            onClick={() =>
              void (site.isError ? site.refetch() : countries.refetch())
            }
          >
            Try again
          </Button>
        </div>
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No country data yet. Some visits cannot be located.
        </p>
      ) : (
        <div className="space-y-3">
          <ol className="space-y-3">
            {rows.map((row) => (
              <li key={row.code} className="min-w-0">
                <div className="flex items-center gap-2 text-sm">
                  <span aria-hidden className="shrink-0 text-lg leading-none">
                    {flag(row.code)}
                  </span>
                  <span className="min-w-0 flex-1 truncate" title={row.name}>
                    {row.name}
                  </span>
                  <span className="shrink-0 text-muted-foreground tabular-nums">
                    {row.pct.toFixed(1)}%
                  </span>
                </div>
                <div
                  className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted"
                  role="img"
                  aria-label={`${row.name}: ${row.visitors} visitors, ${row.pct.toFixed(1)} percent`}
                >
                  <div
                    className="h-full rounded-full bg-primary"
                    style={{ width: `${row.pct}%` }}
                  />
                </div>
              </li>
            ))}
          </ol>
          {countries.hasNextPage ? (
            <Button
              size="sm"
              variant="ghost"
              className="w-full"
              disabled={countries.isFetchingNextPage}
              onClick={() => void countries.fetchNextPage()}
            >
              {countries.isFetchingNextPage
                ? "Loading…"
                : rows.length === 10
                  ? "View all"
                  : "Load more"}
            </Button>
          ) : null}
          <p className="text-xs text-muted-foreground">
            Share of all visitors. Location may be unavailable for some visits.
          </p>
        </div>
      )}
    </CardShell>
  );
}
