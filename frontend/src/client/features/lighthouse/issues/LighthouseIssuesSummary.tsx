import { cn } from "cn";
import { SCORE_TEXT_CLASS, scoreTone } from "@/client/features/audit/shared";
import type { LighthouseMetrics, LighthouseScores } from "./types";

export function LighthouseIssuesSummary({
  scores,
  metrics,
}: {
  scores?: LighthouseScores | null;
  metrics?: LighthouseMetrics | null;
}) {
  const metricItems = getMetricItems(metrics);

  if (!scores && metricItems.length === 0) {
    return null;
  }

  return (
    <>
      {scores ? (
        <div className="grid grid-cols-4 gap-3">
          <ScoreGauge label="Performance" score={scores.performance} />
          <ScoreGauge label="Accessibility" score={scores.accessibility} />
          <ScoreGauge label="Best Practices" score={scores["best-practices"]} />
          <ScoreGauge label="SEO" score={scores.seo} />
        </div>
      ) : null}
      {metricItems.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">
            Lab test: one simulated visit by Google
          </p>
          <ul className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {metricItems.map((metric) => {
              const tone = scoreTone(metric.score);
              return (
                <li
                  key={metric.label}
                  title={metric.meaning}
                  className="rounded-lg border border-border bg-muted/30 px-3 py-2"
                >
                  <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <span
                      aria-hidden
                      className={cn(
                        "size-2 rounded-full",
                        tone ? SCORE_DOT_CLASS[tone] : "bg-border",
                      )}
                    />
                    {metric.label}
                  </span>
                  <span className="block text-base font-semibold tabular-nums">
                    {metric.value}
                  </span>
                  <span className="block text-[11px] leading-tight text-muted-foreground">
                    {metric.meaning}
                  </span>
                </li>
              );
            })}
          </ul>
        </div>
      ) : null}
    </>
  );
}

const SCORE_DOT_CLASS = {
  success: "bg-success",
  warning: "bg-warning",
  destructive: "bg-destructive",
};

const SCORE_STROKE_CLASS = {
  success: "stroke-success",
  warning: "stroke-warning",
  destructive: "stroke-destructive",
};

function ScoreGauge({ label, score }: { label: string; score: number | null }) {
  const displayScore = score ?? 0;
  const tone = scoreTone(score);
  const radius = 28;
  const circumference = 2 * Math.PI * radius;
  const progress = (displayScore / 100) * circumference;

  return (
    <div className="flex flex-col items-center gap-1.5 py-2">
      <div className="relative size-16">
        <svg viewBox="0 0 64 64" className="size-full -rotate-90">
          <circle
            cx="32"
            cy="32"
            r={radius}
            fill="none"
            strokeWidth="4"
            className="stroke-border"
          />
          {tone != null ? (
            <circle
              cx="32"
              cy="32"
              r={radius}
              fill="none"
              strokeWidth="4"
              strokeLinecap="round"
              strokeDasharray={`${progress} ${circumference}`}
              className={SCORE_STROKE_CLASS[tone]}
            />
          ) : null}
        </svg>
        <span
          className={cn(
            "absolute inset-0 flex items-center justify-center text-lg font-bold",
            tone ? SCORE_TEXT_CLASS[tone] : "text-muted-foreground",
          )}
        >
          {score ?? "-"}
        </span>
      </div>
      <span className="text-[11px] text-muted-foreground text-center leading-tight">
        {label}
      </span>
    </div>
  );
}

function getMetricItems(metrics?: LighthouseMetrics | null) {
  if (!metrics) return [];

  return [
    {
      label: "LCP",
      metric: metrics.largestContentfulPaint,
      meaning: "Main content shows",
    },
    {
      label: "CLS",
      metric: metrics.cumulativeLayoutShift,
      meaning: "Page jumps while loading",
    },
    {
      label: "TBT",
      metric: metrics.totalBlockingTime,
      meaning: "Page frozen, cannot react",
    },
    {
      label: "FCP",
      metric: metrics.firstContentfulPaint,
      meaning: "Anything shows",
    },
    {
      label: "Speed Index",
      metric: metrics.speedIndex,
      meaning: "How fast the screen fills",
    },
    {
      label: "TTI",
      metric: metrics.timeToInteractive,
      meaning: "Fully usable",
    },
    {
      label: "INP",
      metric: metrics.interactionToNextPaint,
      meaning: "Reacts to a tap",
    },
    {
      label: "TTFB",
      metric: metrics.serverResponseTime,
      meaning: "Server answers",
    },
  ].flatMap(({ label, metric, meaning }) => {
    // The server-response audit's displayValue is a sentence ("Root document
    // took 420 ms"), too long for a tile, so it is shown from its number.
    const value =
      label === "TTFB" && metric.numericValue != null
        ? `${Math.round(metric.numericValue)} ms`
        : metric.displayValue;
    return value == null
      ? []
      : [{ label, meaning, value, score: metric.score }];
  });
}
