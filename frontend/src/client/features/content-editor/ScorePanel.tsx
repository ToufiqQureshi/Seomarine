import { sortBy } from "remeda";
import { cn } from "cn";
import type { EditorMode, ScoreCheck } from "./scoring";

type Score = {
  score: number;
  checks: ScoreCheck[];
  hasBrief: boolean;
  blocksPublish: boolean;
  capReason?: string;
};

const verdictFor = (score: number) =>
  score >= 80
    ? "Ready to publish"
    : score >= 50
      ? "Getting there"
      : "Needs work";

/**
 * The score and the full checklist for the chosen mode. Every check shows the
 * points it earned out of its weight (from scoring-rules.ts), unfinished ones
 * first, so writers see exactly what moves the score.
 */
export function ScorePanel({
  mode,
  onModeChange,
  seo,
  aeo,
}: {
  mode: EditorMode;
  onModeChange: (mode: EditorMode) => void;
  seo: Score;
  aeo: Score;
}) {
  const active = mode === "seo" ? seo : aeo;
  const checks = sortBy(
    active.checks,
    [(check) => (check.weight === 0 ? 2 : check.passed ? 1 : 0), "asc"],
    [(check) => check.weight - check.earned, "desc"],
  );
  const ringClass =
    active.score >= 80
      ? "text-success"
      : active.score >= 50
        ? "text-warning"
        : "text-destructive";

  return (
    <>
      <section className="rounded-xl border bg-card p-4">
        <div
          role="group"
          aria-label="Score type"
          className="grid grid-cols-2 gap-1 rounded-lg border bg-muted p-1"
        >
          {(
            [
              ["seo", `SEO · ${seo.score}`],
              ["aeo", `AI answers · ${aeo.score}`],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              aria-pressed={mode === value}
              onClick={() => onModeChange(value)}
              className={cn(
                "min-h-9 rounded-md text-sm font-medium",
                mode === value
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="mt-4 flex items-center gap-4" aria-live="polite">
          <span
            className={cn(
              "flex size-17 shrink-0 items-center justify-center rounded-full border-[6px] font-serif text-3xl tabular-nums text-foreground",
              ringClass,
            )}
            style={{ borderColor: "currentColor" }}
          >
            <span className="text-foreground">{active.score}</span>
          </span>
          <div className="min-w-0">
            <p className="font-semibold">{verdictFor(active.score)}</p>
            <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
              {active.hasBrief
                ? "Compared with the pages that rank for your topic."
                : "Basic checks only: no search brief is loaded, so competitor terms and questions are not scored yet."}
            </p>
          </div>
        </div>

        {active.capReason ? (
          <p
            role={active.blocksPublish ? "alert" : undefined}
            className={cn(
              "mt-3 rounded-md px-3 py-2 text-xs leading-5",
              active.blocksPublish
                ? "border border-destructive/40 bg-destructive/10 font-medium text-destructive"
                : "bg-muted text-muted-foreground",
            )}
          >
            {active.capReason}
            {active.blocksPublish
              ? " Publish now is blocked until this is fixed."
              : ""}
          </p>
        ) : null}

        <h3 className="label-mono mt-4 mb-2 text-muted-foreground">
          Checklist · {checks.length} checks
        </h3>
        <ul className="space-y-1">
          {checks.map((check) => (
            <CheckRow key={check.id} check={check} />
          ))}
        </ul>

        <h3 className="label-mono mt-4 mb-2 text-muted-foreground">
          Hard limits
        </h3>
        <ul className="space-y-1 text-xs leading-5 text-muted-foreground">
          <li>
            <strong className="text-foreground">Keyword stuffing</strong> → max
            40, and Publish now is blocked
          </li>
          <li>
            <strong className="text-foreground">Copied from a top page</strong>{" "}
            → max 30 (needs a search brief)
          </li>
          <li>
            <strong className="text-foreground">Under 100 words</strong> → max
            30
          </li>
        </ul>
        <p className="mt-3 text-[11px] text-muted-foreground">
          Writing checks, not a Google ranking prediction.
        </p>
      </section>

      <section className="rounded-xl border bg-card p-4 text-sm leading-6 text-muted-foreground">
        <h3 className="mb-1 font-semibold text-foreground">
          How to write here
        </h3>
        Select text to make it bold or add a link. Type <strong>/</strong> on an
        empty line for headings, lists, images and tables. Paste or drag an
        image in to upload it.
      </section>
    </>
  );
}

function CheckRow({ check }: { check: ScoreCheck }) {
  const notCounted = check.weight === 0;
  const none = !notCounted && check.earned === 0;
  const state = notCounted
    ? {
        mark: "–",
        label: "Not counted",
        tone: "bg-muted text-muted-foreground",
      }
    : check.passed
      ? { mark: "✓", label: "Done", tone: "bg-primary text-primary-foreground" }
      : none
        ? {
            mark: "×",
            label: "Missing",
            tone: "bg-destructive/15 text-destructive",
          }
        : {
            mark: "!",
            label: "Partly done",
            tone: "bg-warning/20 text-warning-foreground",
          };

  return (
    <li
      className={cn(
        "flex gap-2 rounded-lg px-2.5 py-2",
        check.passed || notCounted ? "bg-background" : "bg-muted/60",
      )}
    >
      <span
        aria-label={state.label}
        className={cn(
          "mt-0.5 flex size-[18px] shrink-0 items-center justify-center rounded-full text-[11px] font-bold",
          state.tone,
        )}
      >
        {state.mark}
      </span>
      <div className="min-w-0 flex-1">
        <p className="flex justify-between gap-2 text-sm">
          <span className="font-medium">{check.label}</span>
          <span className="shrink-0 font-mono text-[11px] text-muted-foreground">
            {notCounted
              ? "not counted"
              : `${Math.round(check.earned * 10) / 10} / ${check.weight}`}
          </span>
        </p>
        <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
          {check.detail}
        </p>
      </div>
    </li>
  );
}
