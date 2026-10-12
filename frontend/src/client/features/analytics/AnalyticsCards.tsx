import { Bot } from "lucide-react";
import { Area, AreaChart } from "recharts";
import {
  ChartGrid,
  ChartXAxis,
  ChartYAxis,
} from "@/client/components/ChartAxes";
import { CardShell } from "@/client/components/CardShell";
import { EmptyState } from "@/client/components/EmptyState";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/client/components/ui/chart";
import { formatCount } from "@/client/features/search-performance/SearchPerformanceColumns";
import type { AnalyticsSummary } from "./analyticsApi";
import {
  aiShareOfVisitors,
  aiSourceRows,
  channelRows,
  DEVICE_LABELS,
  formatShare,
  type ShareRow,
} from "./presentation";

const trafficChartConfig = {
  visitors: { label: "Visitors", color: "var(--chart-1)" },
  pageviews: { label: "Pageviews", color: "var(--chart-2)" },
} satisfies ChartConfig;

function formatDay(date: string): string {
  // The API's days are UTC: format in UTC so a day never shifts by one.
  return new Date(`${date}T00:00:00Z`).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  });
}

export function TrafficChartCard({
  series,
}: {
  series: AnalyticsSummary["series"];
}) {
  return (
    <CardShell title="Visitors over time">
      <ChartContainer config={trafficChartConfig} className="h-64">
        <AreaChart data={series} margin={{ top: 8, right: 8, left: 0 }}>
          <ChartGrid />
          <ChartXAxis dataKey="date" tickFormatter={formatDay} />
          <ChartYAxis allowDecimals={false} tickFormatter={formatCount} />
          <ChartTooltip
            content={
              <ChartTooltipContent
                labelFormatter={(label: unknown) =>
                  typeof label === "string" ? formatDay(label) : ""
                }
                valueFormatter={(value) => formatCount(Number(value))}
              />
            }
          />
          <ChartLegend content={<ChartLegendContent />} />
          {(["pageviews", "visitors"] as const).map((key) => (
            <Area
              key={key}
              type="monotone"
              dataKey={key}
              stroke={`var(--color-${key})`}
              strokeWidth={2}
              fill={`var(--color-${key})`}
              fillOpacity={0.08}
            />
          ))}
        </AreaChart>
      </ChartContainer>
    </CardShell>
  );
}

/** Rows of label, count and a bar showing the row's share. */
function ShareList({ rows }: { rows: ShareRow[] }) {
  return (
    <ul className="space-y-3">
      {rows.map((row) => (
        <li key={row.key} className="space-y-1">
          <div className="flex items-baseline justify-between gap-3 text-sm">
            <span className="min-w-0 truncate">
              <span className="font-medium">{row.label}</span>
              {row.hint ? (
                <span className="ml-2 text-xs text-muted-foreground">
                  {row.hint}
                </span>
              ) : null}
            </span>
            <span className="shrink-0 tabular-nums">
              {formatCount(row.visitors)}{" "}
              <span className="text-muted-foreground">
                · {formatShare(row.share)}
              </span>
            </span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-primary"
              style={{ width: `${row.share * 100}%` }}
            />
          </div>
        </li>
      ))}
    </ul>
  );
}

export function AiTrafficCard({
  aiSources,
  channels,
}: Pick<AnalyticsSummary, "aiSources" | "channels">) {
  const rows = aiSourceRows(aiSources);
  const aiVisitors = rows.reduce((sum, row) => sum + row.visitors, 0);
  return (
    <CardShell
      title="AI traffic"
      icon={<Bot className="size-4 text-primary" />}
      stamp={
        aiVisitors > 0
          ? `${formatShare(aiShareOfVisitors(channels))} of your visitors`
          : undefined
      }
    >
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">
          People who find you through ChatGPT and other AI assistants usually
          arrive ready to act, so they often convert better than search
          visitors. We also catch AI visits that other tools file as
          &ldquo;Direct&rdquo;.
        </p>
        {aiVisitors === 0 ? (
          <EmptyState
            icon={Bot}
            size="sm"
            title="No visitors from AI assistants yet"
            description="When ChatGPT, Perplexity, Gemini, Claude or Copilot sends someone to your site, it shows up here."
          />
        ) : (
          <ShareList rows={rows} />
        )}
      </div>
    </CardShell>
  );
}

export function ChannelsCard({ channels }: Pick<AnalyticsSummary, "channels">) {
  const rows = channelRows(channels);
  return (
    <CardShell title="Where visitors come from">
      {rows.length === 0 ? (
        <EmptyState
          variant="plain"
          size="sm"
          title="No visits in this period"
        />
      ) : (
        <ShareList rows={rows} />
      )}
    </CardShell>
  );
}

/** A two-column list: a name and a count, for pages, referrers and devices. */
export function CountListCard({
  title,
  unit,
  rows,
  emptyTitle,
}: {
  title: string;
  unit: string;
  rows: { key: string; label: string; count: number }[];
  emptyTitle: string;
}) {
  return (
    <CardShell title={title}>
      {rows.length === 0 ? (
        <EmptyState variant="plain" size="sm" title={emptyTitle} />
      ) : (
        <table className="w-full text-sm">
          <thead className="sr-only">
            <tr>
              <th>Name</th>
              <th>{unit}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr
                key={row.key}
                className="border-b border-border last:border-0"
              >
                <td className="max-w-0 truncate py-2 pr-3" title={row.label}>
                  {row.label}
                </td>
                <td className="py-2 text-right tabular-nums">
                  {formatCount(row.count)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </CardShell>
  );
}

export function deviceRows(devices: AnalyticsSummary["devices"]) {
  return devices.map((row) => ({
    key: row.device,
    label: DEVICE_LABELS[row.device],
    count: row.visitors,
  }));
}
