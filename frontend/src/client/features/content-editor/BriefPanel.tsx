import { useMutation } from "@tanstack/react-query";
import { RefreshCw, Search } from "lucide-react";
import { cn } from "cn";
import { Button } from "@/client/components/ui/button";
import { briefMatchesTopic, termUsage } from "./brief-view";
import { fetchSearchBrief, type SearchBrief } from "./contentEditorApi";
import { useEditorSession } from "./editor-session";

// About 9 credits per new brief: one SERP plus up to eight page reads. The
// server charges what the provider reports, never above this.
const BRIEF_CREDIT_NOTE = "about 9 credits";

const usageTone = {
  under: "border-warning/40 bg-warning/10",
  "in-range": "border-success/40 bg-success/10",
  over: "border-destructive/40 bg-destructive/10",
} as const;

/**
 * The search brief: what the pages ranking for the topic cover. Building one
 * costs credits, so it only runs when the writer asks; a brief someone else
 * already built for the same topic and market is free.
 */
export function BriefPanel({
  projectId,
  keyword,
  content,
  brief,
  onBrief,
}: {
  projectId: string;
  keyword: string;
  content: string;
  brief: SearchBrief | null;
  onBrief: (brief: SearchBrief) => void;
}) {
  const { editor } = useEditorSession();
  const topic = keyword.trim();
  const mutation = useMutation({
    mutationFn: (refresh: boolean) =>
      fetchSearchBrief({ data: { projectId, keyword: topic, refresh } }),
    onSuccess: onBrief,
    meta: { errorToast: false },
  });
  const current = brief && briefMatchesTopic(brief, topic) ? brief : null;
  const usage = current ? termUsage(current, content) : [];

  if (!current) {
    return (
      <section className="space-y-3 rounded-xl border bg-card p-4">
        <h2 className="font-medium">Search brief</h2>
        <p className="text-sm leading-6 text-muted-foreground">
          See what the pages ranking on Google for your topic cover: the terms
          they use, the questions people ask and how long they are. Your score
          then compares your article with them.
        </p>
        {brief && !current ? (
          <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
            Your topic changed, so the brief for “{brief.keyword}” no longer
            applies.
          </p>
        ) : null}
        <Button
          className="w-full"
          disabled={!topic || mutation.isPending}
          onClick={() => mutation.mutate(false)}
        >
          <Search aria-hidden />
          {mutation.isPending
            ? "Reading the top pages…"
            : `Get search brief (${BRIEF_CREDIT_NOTE})`}
        </Button>
        {!topic ? (
          <p className="text-xs text-muted-foreground">
            Add a target topic first.
          </p>
        ) : (
          <p className="text-xs text-muted-foreground">
            Free if a brief for this topic was made in the last 30 days. Basic
            plan and up.
          </p>
        )}
        {mutation.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {mutation.error instanceof Error
              ? mutation.error.message
              : "Could not get the brief."}
          </p>
        ) : null}
      </section>
    );
  }

  return (
    <section className="space-y-4 rounded-xl border bg-card p-4">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="font-medium">Search brief</h2>
          <p className="text-xs text-muted-foreground">
            “{current.keyword}” · {current.competitors.length} top pages ·{" "}
            {new Date(current.fetchedAt).toLocaleDateString()}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={mutation.isPending}
          onClick={() => {
            if (
              window.confirm(
                `Read the top pages again? This uses ${BRIEF_CREDIT_NOTE}.`,
              )
            )
              mutation.mutate(true);
          }}
        >
          <RefreshCw aria-hidden />
          Refresh
        </Button>
      </div>
      {mutation.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {mutation.error instanceof Error
            ? mutation.error.message
            : "Could not refresh the brief."}
        </p>
      ) : null}

      <div>
        <h3 className="label-mono mb-2 text-muted-foreground">
          Terms · used / range
        </h3>
        {usage.length ? (
          <ul className="flex flex-wrap gap-1.5">
            {usage.map((item) => (
              <li key={item.term}>
                <button
                  type="button"
                  title="Find in the article"
                  className={cn(
                    "rounded-full border px-2.5 py-1 text-xs",
                    usageTone[item.status],
                  )}
                  onClick={() => {
                    if (!editor) return;
                    let range: { from: number; to: number } | undefined;
                    editor.state.doc.descendants((node, position) => {
                      if (range || !node.isText || !node.text) return !range;
                      const index = node.text
                        .toLocaleLowerCase()
                        .indexOf(item.term);
                      if (index >= 0) {
                        range = {
                          from: position + index,
                          to: position + index + item.term.length,
                        };
                      }
                      return !range;
                    });
                    if (range) {
                      editor.chain().focus().setTextSelection(range).run();
                    }
                  }}
                >
                  {item.term}{" "}
                  <span className="font-mono text-muted-foreground">
                    {item.used} / {item.min}–{item.max}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-xs text-muted-foreground">
            The top pages share no clear terms for this topic.
          </p>
        )}
      </div>

      {current.questions.length ? (
        <div>
          <h3 className="label-mono mb-2 text-muted-foreground">
            Questions people ask
          </h3>
          <ul className="space-y-1.5">
            {current.questions.slice(0, 10).map((question) => (
              <li
                key={question}
                className="flex items-start justify-between gap-2 text-sm"
              >
                <span>{question}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!editor}
                  onClick={() =>
                    editor
                      ?.chain()
                      .focus("end")
                      .insertContent(`\n\n## ${question}\n\n`, {
                        contentType: "markdown",
                      })
                      .run()
                  }
                >
                  Add as heading
                </Button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <details>
        <summary className="cursor-pointer text-sm font-medium">
          Top pages ({current.competitors.length})
        </summary>
        <ul className="mt-2 space-y-2">
          {current.competitors.map((competitor) => (
            <li key={competitor.url} className="text-xs">
              <a
                href={competitor.url}
                target="_blank"
                rel="noreferrer"
                className="font-medium text-primary underline-offset-2 hover:underline"
              >
                {competitor.title || new URL(competitor.url).hostname}
              </a>
              <span className="text-muted-foreground">
                {" "}
                · {competitor.wordCount.toLocaleString()} words
              </span>
              {competitor.headings.length ? (
                <p className="mt-0.5 line-clamp-2 text-muted-foreground">
                  {competitor.headings.slice(0, 6).join(" · ")}
                </p>
              ) : null}
            </li>
          ))}
        </ul>
      </details>
    </section>
  );
}
