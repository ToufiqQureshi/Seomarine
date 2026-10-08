import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Code } from "lucide-react";
import { QueryError } from "@/client/components/QueryState";
import { SegmentedToggle } from "@/client/components/SegmentedToggle";
import { SkeletonStatGrid } from "@/client/components/SkeletonPresets";
import { StatTile } from "@/client/components/StatTile";
import { Button } from "@/client/components/ui/button";
import { Card, CardContent } from "@/client/components/ui/card";
import { Skeleton } from "@/client/components/ui/skeleton";
import { ProjectPageHeader } from "@/client/features/projects/ProjectPageHeader";
import { formatCount } from "@/client/features/search-performance/SearchPerformanceColumns";
import {
  analyticsSummaryQueryOptions,
  RANGE_DAYS,
  rangeForDays,
  type AnalyticsSummary,
  type RangeDays,
} from "./analyticsApi";
import {
  AiTrafficCard,
  ChannelsCard,
  CountListCard,
  deviceRows,
  TrafficChartCard,
} from "./AnalyticsCards";
import { aiShareOfVisitors, displayPath, formatShare } from "./presentation";
import { TrackingCodeCard } from "./TrackingCodeCard";
import { CountriesCard } from "./CountriesCard";

const RANGE_ITEMS = RANGE_DAYS.map((days) => ({
  value: `${days}` as const,
  icon: null,
  label: `${days} days`,
}));

export function AnalyticsPage({
  projectId,
  days,
  onDaysChange,
}: {
  projectId: string;
  days: RangeDays;
  onDaysChange: (days: RangeDays) => void;
}) {
  // Day strings: the query key stays the same until the date changes.
  const summaryQuery = useQuery(
    analyticsSummaryQueryOptions(projectId, rangeForDays(days, new Date())),
  );
  const [showCode, setShowCode] = useState(false);
  const summary = summaryQuery.data;
  const hasVisits = summary !== undefined && summary.pageviews > 0;

  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-7xl space-y-6">
        <ProjectPageHeader
          projectId={projectId}
          title="Analytics"
          actions={
            <>
              <SegmentedToggle
                items={RANGE_ITEMS}
                value={`${days}`}
                onChange={(value) => {
                  const picked = RANGE_DAYS.find((d) => `${d}` === value);
                  if (picked) onDaysChange(picked);
                }}
                showLabels
              />
              {hasVisits ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setShowCode((shown) => !shown)}
                  aria-expanded={showCode}
                >
                  <Code />
                  Tracking code
                </Button>
              ) : null}
            </>
          }
        />
        {summaryQuery.isPending ? (
          <DashboardSkeleton />
        ) : summaryQuery.isError && !summary ? (
          <QueryError
            error={summaryQuery.error}
            fallback="We couldn't load your analytics."
            onRetry={() => void summaryQuery.refetch()}
            isRetrying={summaryQuery.isFetching}
          />
        ) : !hasVisits ? (
          <SetupState
            projectId={projectId}
            days={days}
            onCheckAgain={() => void summaryQuery.refetch()}
            isChecking={summaryQuery.isFetching}
          />
        ) : (
          <>
            {showCode ? <TrackingCodeCard projectId={projectId} /> : null}
            <Dashboard
              summary={summary}
              projectId={projectId}
              range={rangeForDays(days, new Date())}
            />
          </>
        )}
      </div>
    </div>
  );
}

function SetupState({
  projectId,
  days,
  onCheckAgain,
  isChecking,
}: {
  projectId: string;
  days: RangeDays;
  onCheckAgain: () => void;
  isChecking: boolean;
}) {
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        No visits in the last {days} days. Install the tracking code to see who
        visits your site, which pages they read, and how many arrive from AI
        assistants.
      </p>
      <TrackingCodeCard projectId={projectId} />
      <Button variant="outline" onClick={onCheckAgain} disabled={isChecking}>
        {isChecking ? "Checking…" : "I've installed it, check again"}
      </Button>
    </div>
  );
}

function Dashboard({
  summary,
  projectId,
  range,
}: {
  summary: AnalyticsSummary;
  projectId: string;
  range: { from: string; to: string };
}) {
  const aiShare = aiShareOfVisitors(summary.channels);
  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="grid grid-cols-2 gap-4 lg:grid-cols-3">
          <StatTile label="Visitors" value={formatCount(summary.visitors)} />
          <StatTile label="Pageviews" value={formatCount(summary.pageviews)} />
          <StatTile
            label="From AI assistants"
            value={formatShare(aiShare)}
            hint="Share of visitors"
          />
        </CardContent>
      </Card>
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <TrafficChartCard series={summary.series} />
        </div>
        <AiTrafficCard
          aiSources={summary.aiSources}
          channels={summary.channels}
        />
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        <ChannelsCard channels={summary.channels} />
        <CountriesCard projectId={projectId} range={range} />
        <CountListCard
          title="Top pages"
          unit="Pageviews"
          emptyTitle="No pages viewed in this period"
          rows={summary.topPages.map((row) => ({
            key: row.path,
            label: displayPath(row.path),
            count: row.pageviews,
          }))}
        />
        <CountListCard
          title="Referring websites"
          unit="Visitors"
          emptyTitle="No visitors came from other websites"
          rows={summary.referrers.map((row) => ({
            key: row.host,
            label: row.host,
            count: row.visitors,
          }))}
        />
        <CountListCard
          title="Devices"
          unit="Visitors"
          emptyTitle="No device data in this period"
          rows={deviceRows(summary.devices)}
        />
      </div>
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="space-y-6" aria-busy>
      <SkeletonStatGrid count={3} className="lg:grid-cols-3" />
      <div className="grid gap-6 lg:grid-cols-3">
        <Skeleton className="h-80 lg:col-span-2" />
        <Skeleton className="h-80" />
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className="h-56" />
        ))}
      </div>
    </div>
  );
}
