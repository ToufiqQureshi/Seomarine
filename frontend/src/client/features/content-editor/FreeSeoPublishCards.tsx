import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@/client/components/ui/button";
import { Checkbox } from "@/client/components/ui/checkbox";
import { apiRequest } from "@/client/lib/uplumoApi";
import { contentAuthorsQueryOptions } from "./authorsApi";
import { fetchLinkSuggestions, applySuggestion } from "./linkSuggestions";
import { useFreeSeoEditor } from "./FreeSeoEditorContext";
import type { ContentDraftInput } from "./scoring";

/**
 * PublishTabPanel is the free-pack block for the editor's Publish tab: the
 * author and review-date card, structured-data toggles plus live preview,
 * and the existing PublishToSite at the end. The side panel mounts it with
 * one line, and every state change goes through the editors' own save flow.
 */ export function PublishTabPanel({
  projectId,
  draftId,
  draft,
  editorBusy,
  publishToSite,
}: {
  projectId: string;
  draftId: string | undefined;
  draft: ContentDraftInput;
  editorBusy: boolean;
  // publishToSite renders the existing publish button block with the
  // schema flags this card owns.
  publishToSite: (schema?: {
    article: boolean;
    faq: boolean;
  }) => React.ReactNode;
}) {
  const [schema, setSchema] = useState({ article: true, faq: true });
  return (
    <div className="space-y-3">
      <AuthorReviewCard
        projectId={projectId}
        draft={draft}
        isBusy={editorBusy}
      />
      <StructuredDataCard
        projectId={projectId}
        draftId={draftId}
        schema={schema}
        onSchemaChange={setSchema}
      />
      {publishToSite(schema)}
    </div>
  );
}

function AuthorReviewCard({
  projectId,
  draft,
  isBusy,
}: {
  projectId: string;
  draft: ContentDraftInput;
  isBusy: boolean;
}) {
  const { setDraftField } = useFreeSeoEditor();
  const authors = useQuery(contentAuthorsQueryOptions(projectId));
  const list = authors.data ?? [];
  return (
    <section className="space-y-3 rounded-lg border border-border bg-card p-4">
      <h2 className="text-sm font-medium">Author and review date</h2>
      {authors.isPending ? (
        <p className="text-xs text-muted-foreground">Loading authors…</p>
      ) : list.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No author profiles yet. Add them on the Integrations settings page.
        </p>
      ) : (
        <select
          aria-label="Draft author"
          className="min-h-9 w-full rounded-md border bg-background px-2 text-sm"
          value={draft.authorId ?? ""}
          disabled={isBusy}
          onChange={(event) => setDraftField("authorId", event.target.value)}
        >
          <option value="">No author</option>
          {list.map((author) => (
            <option key={author.id} value={author.id}>
              {author.name}
              {author.role ? ` · ${author.role}` : ""}
            </option>
          ))}
        </select>
      )}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {draft.lastReviewedAt
            ? `Reviewed ${draft.lastReviewedAt}`
            : "Never marked as reviewed"}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isBusy}
          onClick={() => setDraftField("lastReviewedAt", todayISO())}
        >
          Mark as reviewed today
        </Button>
      </div>
    </section>
  );
}

// todayISO is the review date the writer means: today, in their calendar.
function todayISO(): string {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
}

const schemaPreviewSchema = z.object({
  schemas: z.array(z.unknown()),
  byline: z.string(),
  skipped: z.string(),
});

function StructuredDataCard({
  projectId,
  draftId,
  schema,
  onSchemaChange,
}: {
  projectId: string;
  draftId: string | undefined;
  schema: { article: boolean; faq: boolean };
  onSchemaChange: (next: { article: boolean; faq: boolean }) => void;
}) {
  const preview = useQuery({
    queryKey: ["schemaPreview", projectId, draftId, schema],
    enabled: !!draftId,
    queryFn: () =>
      apiRequest(
        `/api/v1/projects/${encodeURIComponent(projectId)}/content/drafts/${encodeURIComponent(draftId ?? "")}/schema-preview`,
        schemaPreviewSchema,
        "POST",
        { schema },
      ),
    retry: false,
  });
  const blocks = preview.data?.schemas ?? [];
  return (
    <section className="space-y-3 rounded-lg border border-border bg-card p-4">
      <h2 className="text-sm font-medium">Structured data</h2>
      <p className="text-xs text-muted-foreground">
        JSON-LD sent inside the article when it publishes to WordPress. The
        site's host fully controls the rest.
      </p>
      <label className="flex items-center gap-2 text-sm">
        <Checkbox
          checked={schema.article}
          onCheckedChange={(value) =>
            onSchemaChange({ ...schema, article: Boolean(value) })
          }
        />
        Article (BlogPosting)
      </label>
      <label className="flex items-center gap-2 text-sm">
        <Checkbox
          checked={schema.faq}
          onCheckedChange={(value) =>
            onSchemaChange({ ...schema, faq: Boolean(value) })
          }
        />
        FAQ (FAQPage)
      </label>
      {schema.faq ? (
        <p className="text-xs text-muted-foreground">
          FAQPage ships only when the article has an FAQ section whose questions
          and answers match the visible text exactly.
        </p>
      ) : null}
      {draftId && blocks.length > 0 ? (
        <pre className="max-h-48 overflow-auto rounded-md bg-muted p-2 text-[11px] leading-5">
          {JSON.stringify(blocks, null, 2)}
        </pre>
      ) : null}
      {preview.data?.skipped ? (
        <p className="text-xs text-muted-foreground" role="status">
          {preview.data.skipped}
        </p>
      ) : null}
      {!draftId ? (
        <p className="text-xs text-muted-foreground">
          Save the draft to preview it.
        </p>
      ) : (
        <p className="text-xs text-muted-foreground">
          Preview uses the last saved draft. Save edits to update it.
        </p>
      )}
    </section>
  );
}

export function LinkSuggestionsCard({
  projectId,
  draft,
  isBusy,
}: {
  projectId: string;
  draft: ContentDraftInput;
  isBusy: boolean;
}) {
  const { editor } = useFreeSeoEditor();
  const suggestions = useMutation({
    mutationFn: (input: ContentDraftInput) =>
      fetchLinkSuggestions({
        projectId,
        content: input.content,
        targetTopic: input.targetKeyword,
        title: input.title,
        slug: input.slug,
      }),
    meta: { errorToast: false },
  });
  const submitted = suggestions.variables;
  const current =
    submitted &&
    submitted.content === draft.content &&
    submitted.targetKeyword === draft.targetKeyword &&
    submitted.title === draft.title &&
    submitted.slug === draft.slug;
  const list = current ? (suggestions.data ?? []) : [];
  return (
    <details className="group">
      <summary className="cursor-pointer list-none rounded-lg border bg-card px-4 py-3 text-sm font-medium marker:hidden">
        Suggested internal links
        <span className="ml-2 text-xs font-normal text-muted-foreground">
          {suggestions.isPending ? "…" : list.length || ""}
        </span>
      </summary>
      <div className="pt-3">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!draft.content.trim() || isBusy || suggestions.isPending}
          onClick={() => suggestions.mutate(draft)}
        >
          {suggestions.isPending ? "Finding links…" : "Find internal links"}
        </Button>
        {suggestions.isError ? (
          <p role="alert" className="mt-2 text-xs text-destructive">
            Could not load suggestions. Try again.
          </p>
        ) : null}
        {suggestions.isPending ? (
          <p className="text-xs text-muted-foreground">
            Looking for related pages…
          </p>
        ) : !suggestions.data || !current ? (
          <p className="mt-2 text-xs text-muted-foreground">
            Use Find internal links to search your site's pages.
          </p>
        ) : list.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            No related pages found on your site for this draft yet.
          </p>
        ) : (
          <ul className="space-y-2">
            {list.map((suggestion) => (
              <li
                key={`${suggestion.url}-${suggestion.anchor}`}
                className="rounded-md border bg-card p-3 text-sm"
              >
                <p className="font-medium">{suggestion.title}</p>
                <p className="text-xs text-muted-foreground">
                  {suggestion.reason}
                </p>
                <p className="mt-1 truncate text-xs text-muted-foreground">
                  “{suggestion.anchor}”
                </p>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="mt-2"
                  onClick={() => {
                    if (
                      !editor ||
                      !applySuggestion(
                        editor,
                        suggestion.anchor,
                        suggestion.url,
                      )
                    ) {
                      toast.error(
                        "This text changed. Update the suggestion and try again.",
                      );
                    }
                  }}
                >
                  Link it
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </details>
  );
}
