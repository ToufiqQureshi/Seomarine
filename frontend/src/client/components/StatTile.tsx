import { cn } from "cn";

/**
 * One metric: a mono uppercase label over a big serif number.
 *
 * `delta` compares the value with the previous period and shows the percent
 * change. It shows nothing when either side is missing or the previous value
 * is zero.
 */
export function StatTile({
  label,
  value,
  tone,
  delta,
  hint,
}: {
  label: string;
  value: string;
  /** Colors the value, for counts that are good or bad news on their own. */
  tone?: "success" | "destructive";
  delta?: { current: number | null; previous: number | null };
  hint?: string;
}) {
  const percent = delta ? percentChange(delta.current, delta.previous) : null;
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <p className="label-mono text-muted-foreground">{label}</p>
      <p
        className={cn(
          "font-serif text-5xl leading-none tabular-nums",
          tone === "success" && "text-success",
          tone === "destructive" && "text-destructive",
        )}
      >
        {value}
      </p>
      {percent === null ? null : (
        <p
          className={cn(
            "font-mono text-xs tabular-nums",
            percent > 0 && "text-success",
            percent < 0 && "text-destructive",
          )}
        >
          {percent > 0 ? "▲ " : percent < 0 ? "▼ " : ""}
          {Math.abs(percent)}%
        </p>
      )}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  );
}

function percentChange(
  current: number | null,
  previous: number | null,
): number | null {
  if (current === null || previous === null || previous <= 0) return null;
  return Math.round(((current - previous) / previous) * 100);
}
