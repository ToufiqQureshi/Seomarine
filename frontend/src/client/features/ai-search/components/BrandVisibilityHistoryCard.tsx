import { useMemo } from "react";
import { AlertTriangle, TrendingUp } from "lucide-react";
import { Line, LineChart } from "recharts";
import {
  ChartGrid,
  ChartXAxis,
  ChartYAxis,
} from "@/client/components/ChartAxes";
import { CardShell } from "@/client/components/CardShell";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/client/components/ui/chart";
import type { BrandVisibilitySnapshot } from "@/types/schemas/ai-search";
import { formatCount } from "@/client/features/search-performance/SearchPerformanceColumns";

const chartConfig = {
  mentions: { label: "AI mentions", color: "var(--chart-1)" },
} satisfies ChartConfig;

function periodLabel(period: string) {
  const date = new Date(`${period}-01T00:00:00.000Z`);
  return date.toLocaleDateString(undefined, {
    month: "short",
    year: "2-digit",
    timeZone: "UTC",
  });
}

export function BrandVisibilityHistoryCard({
  snapshots,
  isLoading,
  isError,
}: {
  snapshots: BrandVisibilitySnapshot[] | undefined;
  isLoading: boolean;
  isError: boolean;
}) {
  const chartData = useMemo(
    () =>
      (snapshots ?? []).map((snapshot) => ({
        period: snapshot.period,
        mentions: snapshot.totalMentions,
      })),
    [snapshots],
  );
  const latest = snapshots?.at(-1);
  const previous = snapshots?.at(-2);
  const mentionsDrop =
    latest && previous && previous.totalMentions > 0
      ? ((previous.totalMentions - latest.totalMentions) /
          previous.totalMentions) *
        100
      : 0;

  return (
    <CardShell
      title="AI visibility history"
      icon={<TrendingUp className="size-4" />}
    >
      {isLoading ? (
        <p className="py-8 text-sm text-muted-foreground">
          Loading saved monthly snapshots…
        </p>
      ) : isError ? (
        <p role="status" className="py-8 text-sm text-muted-foreground">
          History is temporarily unavailable. The current lookup is still
          available.
        </p>
      ) : chartData.length === 0 ? (
        <p className="py-8 text-sm text-muted-foreground">
          No monthly snapshots yet. A snapshot is recorded after a complete,
          user-run lookup. Run another lookup in a later month to build a trend.
        </p>
      ) : (
        <div className="space-y-3">
          {mentionsDrop >= 30 ? (
            <div className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning/10 p-3 text-sm">
              <AlertTriangle className="mt-0.5 size-4 shrink-0 text-warning-foreground" />
              <p>
                AI mention estimates are down {mentionsDrop.toFixed(0)}% from
                the previous saved month. Provider estimates can change; rerun
                next month to confirm a trend.
              </p>
            </div>
          ) : null}
          {chartData.length >= 2 ? (
            <ChartContainer config={chartConfig} className="h-64 w-full">
              <LineChart
                data={chartData}
                margin={{ top: 8, right: 8, left: 0, bottom: 0 }}
              >
                <ChartGrid />
                <ChartXAxis
                  dataKey="period"
                  tickFormatter={(value: string) => periodLabel(value)}
                />
                <ChartYAxis
                  allowDecimals={false}
                  tickFormatter={(value: number) => formatCount(value)}
                />
                <ChartTooltip
                  content={
                    <ChartTooltipContent
                      labelFormatter={(label: unknown) =>
                        typeof label === "string" ? periodLabel(label) : ""
                      }
                      valueFormatter={(value) => formatCount(Number(value))}
                    />
                  }
                />
                <Line
                  type="monotone"
                  dataKey="mentions"
                  stroke="var(--color-mentions)"
                  strokeWidth={2}
                  dot
                />
              </LineChart>
            </ChartContainer>
          ) : (
            <p className="py-6 text-sm text-muted-foreground">
              One snapshot saved for{" "}
              {periodLabel(latest?.period ?? chartData[0].period)}:{" "}
              {formatCount(latest?.totalMentions ?? chartData[0].mentions)}{" "}
              mentions and {formatCount(latest?.citedPages ?? 0)} cited pages. A
              trend appears after another month is saved.
            </p>
          )}
          {latest ? (
            <div className="grid grid-cols-3 gap-3 border-t pt-3 text-sm">
              <div>
                <p className="text-xs text-muted-foreground">AI mentions</p>
                <p className="font-semibold">
                  {formatCount(latest.totalMentions)}
                </p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Search volume</p>
                <p className="font-semibold">
                  {formatCount(latest.totalAiSearchVolume)}
                </p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Cited pages</p>
                <p className="font-semibold">
                  {formatCount(latest.citedPages)}
                </p>
              </div>
            </div>
          ) : null}
          <p className="text-xs text-muted-foreground">
            Data is saved once per month from a complete manual lookup. This
            chart uses stored results and makes no extra provider calls.
          </p>
        </div>
      )}
    </CardShell>
  );
}
