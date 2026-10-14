import { CircleCheck, CircleX, Users } from "lucide-react";
import { cn } from "cn";
import { Badge } from "@/client/components/ui/badge";
import type { LighthouseFieldData } from "./types";

type FieldMetric = NonNullable<LighthouseFieldData>["metrics"][number];

// Plain names and one-line meanings, so a customer can read the card without
// knowing the acronyms.
const FIELD_METRIC_COPY: Record<
  FieldMetric["key"],
  { name: string; meaning: string }
> = {
  lcp: {
    name: "Largest Contentful Paint (LCP)",
    meaning: "How long until the main content shows",
  },
  inp: {
    name: "Interaction to Next Paint (INP)",
    meaning: "How fast the page reacts to a tap or click",
  },
  cls: {
    name: "Cumulative Layout Shift (CLS)",
    meaning: "How much the page jumps around while loading",
  },
  fcp: {
    name: "First Contentful Paint (FCP)",
    meaning: "How long until anything shows",
  },
  ttfb: {
    name: "Time to First Byte (TTFB)",
    meaning: "How long the server takes to answer",
  },
};

const CATEGORY_BADGE: Record<
  string,
  { label: string; variant: "success" | "warning" | "destructive" }
> = {
  FAST: { label: "Good", variant: "success" },
  AVERAGE: { label: "Needs work", variant: "warning" },
  SLOW: { label: "Poor", variant: "destructive" },
};

/**
 * What real Chrome visitors experienced over the last 28 days (Google's CrUX
 * data, sent with every PageSpeed check). This is what Google uses for
 * ranking, unlike the lab scores, which come from one simulated visit.
 */
export function LighthouseFieldData({
  fieldData,
}: {
  fieldData: LighthouseFieldData | undefined;
}) {
  if (fieldData === undefined) return null;

  return (
    <section className="space-y-3 rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="space-y-0.5">
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            <Users className="size-4 text-muted-foreground" />
            Real visitors, last 28 days
          </h2>
          <p className="text-xs text-muted-foreground">
            {fieldData == null
              ? "Google has no real-visitor data for this site yet. That is normal for new or low-traffic sites; the lab test below still applies."
              : fieldData.scope === "origin"
                ? "This page has too few visits of its own, so this is your whole site's data, from Chrome users."
                : "From Chrome users who opened this page. Google uses this data for ranking."}
          </p>
        </div>
        {fieldData ? (
          <CoreWebVitalsBadge status={fieldData.coreWebVitals} />
        ) : null}
      </div>

      {fieldData && fieldData.metrics.length > 0 ? (
        <>
          <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {fieldData.metrics.map((metric) => (
              <FieldMetricItem key={metric.key} metric={metric} />
            ))}
          </ul>
          <DistributionLegend />
        </>
      ) : null}
    </section>
  );
}

function CoreWebVitalsBadge({ status }: { status: "passed" | "failed" }) {
  const passed = status === "passed";
  return (
    <Badge variant={passed ? "success" : "destructive"}>
      {passed ? <CircleCheck /> : <CircleX />}
      Core Web Vitals {passed ? "passed" : "failed"}
    </Badge>
  );
}

function FieldMetricItem({ metric }: { metric: FieldMetric }) {
  const copy = FIELD_METRIC_COPY[metric.key];
  const badge = CATEGORY_BADGE[metric.category];

  return (
    <li className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-xs font-medium">{copy.name}</span>
        {badge ? <Badge variant={badge.variant}>{badge.label}</Badge> : null}
      </div>
      <p className="text-2xl font-semibold tabular-nums">
        {formatFieldValue(metric)}
      </p>
      <p className="text-xs text-muted-foreground">{copy.meaning}</p>
      <DistributionBar metric={metric} />
    </li>
  );
}

const SEGMENTS = [
  { key: "good", label: "Good", className: "bg-success" },
  { key: "needsImprovement", label: "Needs work", className: "bg-warning" },
  { key: "poor", label: "Poor", className: "bg-destructive" },
] as const;

/** Share of visits in each of Google's bands, as one stacked bar. */
function DistributionBar({ metric }: { metric: FieldMetric }) {
  const label = SEGMENTS.map(
    (segment) => `${segment.label} ${formatShare(metric[segment.key])}`,
  ).join(", ");

  return (
    <div className="space-y-1">
      <div
        role="img"
        aria-label={`Share of visits: ${label}`}
        className="flex h-2 gap-0.5 overflow-hidden rounded-full"
      >
        {SEGMENTS.map((segment) =>
          metric[segment.key] > 0 ? (
            <div
              key={segment.key}
              title={`${segment.label}: ${formatShare(metric[segment.key])} of visits`}
              className={cn(
                "h-full first:rounded-l-full last:rounded-r-full",
                segment.className,
              )}
              style={{ width: `${metric[segment.key] * 100}%` }}
            />
          ) : null,
        )}
      </div>
      <p className="text-[11px] tabular-nums text-muted-foreground">
        {formatShare(metric.good)} of visits were good
      </p>
    </div>
  );
}

function DistributionLegend() {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
      {SEGMENTS.map((segment) => (
        <span key={segment.key} className="flex items-center gap-1.5">
          <span className={cn("size-2 rounded-full", segment.className)} />
          {segment.label}
        </span>
      ))}
      <span>Values are what 3 in 4 visits got or better.</span>
    </div>
  );
}

function formatFieldValue(metric: FieldMetric) {
  if (metric.key === "cls") return metric.percentile.toFixed(2);
  if (metric.percentile >= 1000)
    return `${(metric.percentile / 1000).toFixed(1)} s`;
  return `${Math.round(metric.percentile)} ms`;
}

function formatShare(share: number) {
  return `${Math.round(share * 100)}%`;
}
