import { Check, CircleHelp, Plus, Trash2, X } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Button } from "@/client/components/ui/button";
import type { ContentDraftSummary } from "./contentEditorApi";
import type { ContentDraftInput, EditorMode, ScoreCheck } from "./scoring";

const reminderStorageKey = (projectId: string) =>
  `uplumo:content-editor:writer-reminder:${projectId}`;

export function WriterReminder({ projectId }: { projectId: string }) {
  const [dismissed, setDismissed] = useState(() => {
    if (typeof window === "undefined") return false;
    try {
      return window.sessionStorage.getItem(reminderStorageKey(projectId)) === "dismissed";
    } catch {
      return false;
    }
  });

  if (dismissed) return null;

  return (
    <aside className="flex items-start justify-between gap-3 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2.5">
      <p className="text-sm leading-5 text-muted-foreground">
        <span className="font-medium text-foreground">Use your own expertise.</span>{" "}
        Let AI help with the draft, add your real examples, and verify every claim.
      </p>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label="Dismiss writing reminder"
        onClick={() => {
          setDismissed(true);
          try {
            window.sessionStorage.setItem(reminderStorageKey(projectId), "dismissed");
          } catch {
            // Dismissal still works for this render if storage is unavailable.
          }
        }}
      >
        <X aria-hidden />
      </Button>
    </aside>
  );
}

export function WritingModeToggle({
  mode,
  onChange,
}: {
  mode: EditorMode;
  onChange: (mode: EditorMode) => void;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-card px-4 py-3">
      <div>
        <p className="label-mono text-muted-foreground">Writing mode</p>
        <p className="text-sm text-muted-foreground">
          Switch the checklist without changing your draft.
        </p>
      </div>
      <div
        className="inline-flex rounded-lg border bg-background p-1"
        role="group"
        aria-label="Choose writing score mode"
      >
        {(["seo", "aeo"] as const).map((option) => (
          <button
            key={option}
            type="button"
            aria-pressed={mode === option}
            onClick={() => onChange(option)}
            className={`rounded-md px-4 py-2 text-sm font-medium capitalize transition-colors ${mode === option ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground"}`}
          >
            {option === "seo" ? "SEO score" : "AEO score"}
          </button>
        ))}
      </div>
    </div>
  );
}

export function ReadinessPanel({
  mode,
  seo,
  aeo,
  activeScore,
  checks,
}: {
  mode: EditorMode;
  seo: { score: number };
  aeo: { score: number };
  activeScore: number;
  checks: ScoreCheck[];
}) {
  return (
    <>
      <section className="rounded-xl border bg-card p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="label-mono text-muted-foreground">
              {mode.toUpperCase()} READINESS
            </p>
            <p
              className="mt-1 font-serif text-6xl tabular-nums"
              aria-live="polite"
            >
              {activeScore}
              <span className="text-2xl text-muted-foreground">/100</span>
            </p>
          </div>
          <div className="rounded-lg bg-muted px-3 py-2 text-center">
            <p className="text-xs text-muted-foreground">SEO</p>
            <p className="font-mono text-lg">{seo.score}</p>
          </div>
          <div className="rounded-lg bg-muted px-3 py-2 text-center">
            <p className="text-xs text-muted-foreground">AEO</p>
            <p className="font-mono text-lg">{aeo.score}</p>
          </div>
        </div>
        <p className="mt-3 text-xs leading-5 text-muted-foreground">
          Directional writing checks, not a Google ranking prediction. Scores
          update as you write.
        </p>
      </section>
      <section className="rounded-xl border bg-card p-4">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="font-medium">{mode.toUpperCase()} checklist</h2>
          <CircleHelp
            className="size-4 text-muted-foreground"
            aria-label="Each check contributes to the score based on its weight."
          />
        </div>
        <ul className="space-y-3">
          {checks.map((check) => (
            <CheckItem key={check.id} check={check} />
          ))}
        </ul>
      </section>
    </>
  );
}

export function SearchPreview({ draft }: { draft: ContentDraftInput }) {
  return (
    <section className="rounded-xl border bg-card p-4">
      <h2 className="mb-3 font-medium">Search preview</h2>
      <div className="rounded-lg border bg-background p-4">
        <p className="truncate text-xs text-muted-foreground">
          {draft.slug
            ? `https://your-site.com/${draft.slug}`
            : "https://your-site.com/your-page"}
        </p>
        <p className="mt-1 line-clamp-2 text-lg leading-6 text-sky-700 dark:text-sky-400">
          {draft.title || "Your page title will appear here"}
        </p>
        <p className="mt-1 line-clamp-2 text-sm leading-5 text-muted-foreground">
          {draft.metaDescription ||
            "Add a search description to preview how your page may appear in search results."}
        </p>
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        Search engines may choose a different title or snippet.
      </p>
    </section>
  );
}

export function DraftsPanel({
  drafts,
  isLoading,
  hasError,
  currentDraftId,
  isBusy,
  onNew,
  onSelect,
  onDelete,
}: {
  drafts: ContentDraftSummary[];
  isLoading: boolean;
  hasError: boolean;
  currentDraftId?: string;
  isBusy: boolean;
  onNew: () => void;
  onSelect: (draft: ContentDraftSummary) => void;
  onDelete: (draft: ContentDraftSummary) => void;
}) {
  return (
    <section className="rounded-xl border bg-card p-4">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 className="font-medium">Your drafts</h2>
        <Button variant="outline" size="sm" disabled={isBusy} onClick={onNew}>
          <Plus aria-hidden />
          New
        </Button>
      </div>
      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading drafts?</p>
      ) : null}
      {hasError ? (
        <p className="text-sm text-destructive">Could not load saved drafts.</p>
      ) : null}
      {drafts.length === 0 && !isLoading ? (
        <p className="text-sm text-muted-foreground">
          Saved drafts will appear here.
        </p>
      ) : null}
      <ul className="space-y-1">
        {drafts.map((saved) => (
          <li
            key={saved.id}
            className={`flex items-center gap-2 rounded-lg px-2 py-2 ${saved.id === currentDraftId ? "bg-muted" : ""}`}
          >
            <button
              type="button"
              disabled={isBusy}
              onClick={() => onSelect(saved)}
              className="min-w-0 flex-1 text-left disabled:opacity-60"
            >
              <span className="block truncate text-sm">
                {saved.title || "Untitled draft"}
              </span>
              <span className="block text-xs text-muted-foreground">
                {new Date(saved.updatedAt).toLocaleDateString()}
              </span>
            </button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Delete ${saved.title || "untitled draft"}`}
              disabled={isBusy}
              onClick={() => onDelete(saved)}
            >
              <Trash2 aria-hidden />
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}

function CheckItem({ check }: { check: ScoreCheck }) {
  const Icon = check.passed ? Check : X;
  return (
    <li className="flex gap-2.5">
      <Icon
        className={`mt-0.5 size-4 shrink-0 ${check.passed ? "text-success" : "text-muted-foreground"}`}
        aria-hidden
      />
      <div className="min-w-0">
        <p className="text-sm font-medium">
          {check.label}
          <span className="ml-1.5 font-mono text-xs text-muted-foreground">
            +{check.weight}
          </span>
        </p>
        <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
          {check.detail}
        </p>
      </div>
    </li>
  );
}

export function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint: string;
  children: ReactNode;
}) {
  return (
    <div className="block space-y-2">
      <label
        htmlFor={id}
        className="flex items-baseline justify-between gap-2 text-sm font-medium"
      >
        {label}
        <span className="text-xs font-normal text-muted-foreground">
          {hint}
        </span>
      </label>
      {children}
    </div>
  );
}
