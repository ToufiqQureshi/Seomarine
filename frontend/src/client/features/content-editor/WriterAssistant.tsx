import { useMutation, useQuery } from "@tanstack/react-query";
import { LoaderCircle, Sparkles } from "lucide-react";
import { useState } from "react";
import { Button } from "@/client/components/ui/button";
import { Textarea } from "@/client/components/ui/textarea";
import {
  generateWritingSuggestion,
  getContentWriterStatus,
} from "./contentEditorApi";
import type { ContentDraftInput, EditorMode } from "./scoring";
import { CreditCostHint } from "@/client/features/billing/credits/CreditCostHint";
import { useEditorSession } from "./editor-session";

const actions = [
  ["outline", "Create outline"],
  ["write_section", "Write this section"],
  ["fill_gap", "Fill a gap"],
  ["answer_first", "Answer first"],
  ["fix_top_issue", "Fix top issue"],
  ["title_meta", "Title and meta options"],
  ["faq", "FAQ from brief questions"],
  ["destuff", "Remove repetition"],
  ["fact_check", "Fact check"],
  ["tone_language", "Tone and language"],
] as const;

export function WriterAssistant({
  projectId,
  draft,
  mode,
  disabled,
  onUse,
}: {
  projectId: string;
  draft: ContentDraftInput;
  mode: EditorMode;
  disabled: boolean;
  onUse: (content: string) => void;
}) {
  const { editor } = useEditorSession();
  const [instructions, setInstructions] = useState("");
  const [language, setLanguage] = useState("");
  const [result, setResult] = useState("");
  const [error, setError] = useState("");
  const availability = useQuery({
    queryKey: ["contentWriterStatus"],
    queryFn: getContentWriterStatus,
    staleTime: 60_000,
  });
  const writerDisabled =
    availability.isPending ||
    availability.isError ||
    !availability.data?.configured;
  const mutation = useMutation({
    mutationFn: (action: (typeof actions)[number][0]) =>
      generateWritingSuggestion({
        data: {
          projectId,
          action,
          mode,
          title: draft.title,
          targetKeyword: draft.targetKeyword,
          content: draft.content,
          language,
          instructions,
        },
      }),
    onSuccess: (suggestion) => {
      setResult(suggestion.content);
      setError("");
    },
    onError: (cause) =>
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not generate a suggestion.",
      ),
  });

  return (
    <section className="space-y-3 rounded-xl border bg-card p-4">
      <div className="flex items-center gap-2">
        <Sparkles aria-hidden className="size-4 text-primary" />
        <h2 className="font-medium">AI writing assistant</h2>
      </div>
      <p className="text-xs leading-5 text-muted-foreground">
        Choose an action to send this brief and article to our AI provider.
        Usage is charged to your organization’s AI credits. Review suggestions
        before using them; generated facts may need verification.
      </p>
      {writerDisabled ? (
        <p
          role="status"
          className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground"
        >
          {availability.isError
            ? "Could not check AI writing availability. Try again shortly."
            : availability.data?.message || "Checking AI writing availability…"}
        </p>
      ) : null}
      <label className="block space-y-1 text-sm">
        <span>
          Output language{" "}
          <span className="text-muted-foreground">(optional)</span>
        </span>
        <input
          id="content-assistant-language"
          name="content-assistant-language"
          value={language}
          onChange={(event) => setLanguage(event.target.value)}
          maxLength={80}
          placeholder="Use the language of my draft"
          className="h-9 w-full rounded-md border bg-background px-3 text-sm"
          disabled={disabled || mutation.isPending}
        />
      </label>
      <label className="block space-y-1 text-sm">
        <span>
          Instructions <span className="text-muted-foreground">(optional)</span>
        </span>
        <Textarea
          id="content-assistant-instructions"
          name="content-assistant-instructions"
          value={instructions}
          onChange={(event) => setInstructions(event.target.value)}
          maxLength={2_000}
          rows={3}
          placeholder="Audience, tone, or facts to emphasize"
          disabled={disabled || mutation.isPending}
        />
      </label>
      <div className="grid grid-cols-2 gap-2">
        {actions.map(([action, label]) => (
          <Button
            key={action}
            type="button"
            size="sm"
            variant="outline"
            disabled={
              disabled ||
              writerDisabled ||
              mutation.isPending ||
              (action !== "outline" && !draft.content.trim())
            }
            onClick={() => mutation.mutate(action)}
          >
            {mutation.isPending && mutation.variables === action ? (
              <LoaderCircle aria-hidden className="animate-spin" />
            ) : null}
            {label}
          </Button>
        ))}
      </div>
      <CreditCostHint feature="ai_writing" />
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {result ? (
        <div className="space-y-2 border-t pt-3">
          <label
            htmlFor="content-ai-suggestion"
            className="text-sm font-medium"
          >
            AI suggestion · review before using
          </label>
          <Textarea
            id="content-ai-suggestion"
            value={result}
            onChange={(event) => setResult(event.target.value)}
            className="min-h-48 font-mono text-xs leading-5"
            maxLength={30_000}
          />
          <Button
            type="button"
            size="sm"
            disabled={disabled || !result.trim()}
            onClick={() => {
              if (editor) {
                editor
                  .chain()
                  .focus()
                  .insertContent(result, { contentType: "markdown" })
                  .run();
              } else {
                onUse(result);
              }
            }}
          >
            Accept suggestion
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              setResult("");
              mutation.reset();
              setError("");
            }}
          >
            Discard
          </Button>
          {mutation.variables ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={disabled || mutation.isPending}
              onClick={() => mutation.mutate(mutation.variables)}
            >
              Try again
            </Button>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
