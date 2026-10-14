import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PublishToSite } from "@/client/features/site-publishing/PublishToSite";
import { Save } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/client/components/ui/button";
import { Input } from "@/client/components/ui/input";
import { Textarea } from "@/client/components/ui/textarea";
import {
  deleteContentDraft,
  getContentDraft,
  listContentDrafts,
  saveContentDraft,
  type ContentDraft,
  type ContentDraftSummary,
} from "./contentEditorApi";
import {
  scoreContent,
  type ContentDraftInput,
  type EditorMode,
} from "./scoring";
import {
  DraftsPanel,
  ReadinessPanel,
  SearchPreview,
  Field,
  WritingModeToggle,
} from "./EditorPanels";
import { formatArticleContent, type ArticleFormat } from "./formatting";
import { WriterAssistant } from "./WriterAssistant";

const blankDraft: ContentDraftInput = {
  title: "",
  targetKeyword: "",
  slug: "",
  metaDescription: "",
  content: "",
};

const queryKey = (projectId: string) => ["contentDrafts", projectId] as const;
const asInput = (draft: ContentDraft): ContentDraftInput => ({
  title: draft.title,
  targetKeyword: draft.targetKeyword,
  slug: draft.slug,
  metaDescription: draft.metaDescription,
  content: draft.content,
});
const wordCount = (text: string) =>
  text.trim() ? (text.trim().match(/[\p{L}\p{N}]+/gu)?.length ?? 0) : 0;
const confirmSuggestionReplacement = (content: string) =>
  !content.trim() ||
  window.confirm("Replace the article with this AI suggestion?");

export function ContentEditorPage({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const draftsQuery = useQuery({
    queryKey: queryKey(projectId),
    queryFn: () => listContentDrafts({ data: { projectId } }),
  });
  const [mode, setMode] = useState<EditorMode>("seo");
  const [draft, setDraft] = useState<ContentDraftInput>(blankDraft);
  const [draftId, setDraftId] = useState<string>();
  const [savedSnapshot, setSavedSnapshot] = useState(blankDraft);
  const [initialized, setInitialized] = useState(false);
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const dirty = JSON.stringify(draft) !== JSON.stringify(savedSnapshot);
  const seo = useMemo(() => scoreContent("seo", draft), [draft]);
  const aeo = useMemo(() => scoreContent("aeo", draft), [draft]);
  const activeScore = mode === "seo" ? seo : aeo;

  useEffect(() => {
    if (draftsQuery.isPending || initialized || !draftsQuery.data) return;
    setInitialized(true);
  }, [draftsQuery.data, draftsQuery.isPending, initialized]);

  const saveMutation = useMutation({
    mutationFn: () =>
      saveContentDraft({
        data: { ...draft, projectId, ...(draftId ? { id: draftId } : {}) },
      }),
    onSuccess: (saved) => {
      const snapshot = asInput(saved);
      setDraftId(saved.id);
      setDraft(snapshot);
      setSavedSnapshot(snapshot);
      void queryClient.invalidateQueries({ queryKey: queryKey(projectId) });
      toast.success("Draft saved");
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : "Could not save this draft. Your writing is still here.",
      ),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) =>
      deleteContentDraft({ data: { projectId, draftId: id } }),
    onSuccess: async (_, id) => {
      if (id === draftId) {
        setDraftId(undefined);
        setDraft(blankDraft);
        setSavedSnapshot(blankDraft);
      }
      await queryClient.invalidateQueries({ queryKey: queryKey(projectId) });
      toast.success("Draft deleted");
    },
    onError: (error) =>
      toast.error(
        error instanceof Error ? error.message : "Could not delete this draft.",
      ),
  });

  const openMutation = useMutation({
    mutationFn: (id: string) =>
      getContentDraft({ data: { projectId, draftId: id } }),
    onSuccess: (saved) => {
      const snapshot = asInput(saved);
      setDraftId(saved.id);
      setDraft(snapshot);
      setSavedSnapshot(snapshot);
    },
    onError: (error) =>
      toast.error(
        error instanceof Error ? error.message : "Could not open this draft.",
      ),
  });
  const editorBusy =
    saveMutation.isPending ||
    openMutation.isPending ||
    deleteMutation.isPending;

  const startNewDraft = () => {
    if (
      dirty &&
      !window.confirm("Discard your unsaved changes and start a new draft?")
    )
      return;
    setDraftId(undefined);
    setDraft(blankDraft);
    setSavedSnapshot(blankDraft);
    setInitialized(true);
  };

  const selectDraft = (saved: ContentDraftSummary) => {
    if (saved.id === draftId) return;
    if (
      dirty &&
      !window.confirm("Discard your unsaved changes and open this draft?")
    )
      return;
    openMutation.mutate(saved.id);
  };

  const update = (field: keyof ContentDraftInput, value: string) =>
    setDraft((current) => ({ ...current, [field]: value }));

  const formatArticle = (format: ArticleFormat) => {
    const editor = editorRef.current;
    const start = editor?.selectionStart ?? draft.content.length;
    const end = editor?.selectionEnd ?? start;
    const result = formatArticleContent(draft.content, start, end, format);
    update("content", result.content);
    requestAnimationFrame(() => {
      editor?.focus();
      editor?.setSelectionRange(result.selectionStart, result.selectionEnd);
    });
  };

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_350px]">
      <section className="min-w-0 space-y-5">
        <WritingModeToggle mode={mode} onChange={setMode} />

        <div className="space-y-5 rounded-xl border bg-card p-4 md:p-6">
          <Field
            id="content-draft-title"
            label="Article title"
            hint={`${draft.title.length}/180`}
          >
            <Input
              id="content-draft-title"
              disabled={editorBusy}
              maxLength={180}
              value={draft.title}
              onChange={(event) => update("title", event.target.value)}
              placeholder="What should readers learn?"
            />
          </Field>
          <div className="grid gap-4 md:grid-cols-2">
            <Field
              id="content-draft-keyword"
              label="Target keyword"
              hint="The main search topic"
            >
              <Input
                id="content-draft-keyword"
                disabled={editorBusy}
                maxLength={120}
                value={draft.targetKeyword}
                onChange={(event) =>
                  update("targetKeyword", event.target.value)
                }
                placeholder="e.g. technical SEO"
              />
            </Field>
            <Field
              id="content-draft-slug"
              label="Page URL slug"
              hint="Lowercase, short, readable"
            >
              <Input
                id="content-draft-slug"
                disabled={editorBusy}
                maxLength={300}
                value={draft.slug}
                onChange={(event) => update("slug", event.target.value)}
                placeholder="technical-seo-guide"
              />
            </Field>
          </div>
          <Field
            id="content-draft-meta"
            label="Search description"
            hint={`${draft.metaDescription.length}/320`}
          >
            <Textarea
              id="content-draft-meta"
              disabled={editorBusy}
              maxLength={320}
              rows={3}
              value={draft.metaDescription}
              onChange={(event) =>
                update("metaDescription", event.target.value)
              }
              placeholder="Summarize the page clearly for searchers."
            />
          </Field>
          <Field
            id="content-draft-body"
            label="Your article"
            hint="Markdown headings, lists, and links are supported"
          >
            <div
              className="flex flex-wrap gap-1"
              role="toolbar"
              aria-label="Article formatting"
            >
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={editorBusy}
                onClick={() => formatArticle("heading")}
              >
                Heading
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={editorBusy}
                onClick={() => formatArticle("bold")}
              >
                <strong>B</strong>
                <span className="sr-only">Bold</span>
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={editorBusy}
                onClick={() => formatArticle("list")}
              >
                Bullet list
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={editorBusy}
                onClick={() => formatArticle("link")}
              >
                Link
              </Button>
            </div>
            <Textarea
              ref={editorRef}
              id="content-draft-body"
              disabled={editorBusy}
              maxLength={150_000}
              className="min-h-[440px] resize-y font-mono leading-7"
              value={draft.content}
              onChange={(event) => update("content", event.target.value)}
              placeholder={`Start with the answer your reader needs.\n\n## Add a clear section\n\nExplain the idea in your own words...`}
            />
          </Field>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
            <p className="text-sm text-muted-foreground">
              {wordCount(draft.content).toLocaleString()} words ·{" "}
              {dirty ? "Unsaved changes" : "All changes saved"}
            </p>
            <Button
              onClick={() => saveMutation.mutate()}
              disabled={
                editorBusy || (!draft.title.trim() && !draft.content.trim())
              }
            >
              <Save aria-hidden />
              {saveMutation.isPending ? "Saving…" : "Save draft"}
            </Button>
          </div>
        </div>
      </section>

      <aside className="space-y-4">
        <ReadinessPanel
          mode={mode}
          seo={seo}
          aeo={aeo}
          activeScore={activeScore.score}
          checks={activeScore.checks}
        />
        <PublishToSite
          projectId={projectId}
          draftId={draftId}
          hasUnsavedChanges={dirty}
        />
        <WriterAssistant
          projectId={projectId}
          draft={draft}
          mode={mode}
          disabled={editorBusy}
          onUse={(content) =>
            confirmSuggestionReplacement(draft.content) &&
            update("content", content)
          }
        />
        <SearchPreview draft={draft} />
        <DraftsPanel
          drafts={draftsQuery.data ?? []}
          isLoading={draftsQuery.isPending}
          hasError={draftsQuery.isError}
          currentDraftId={draftId}
          isBusy={editorBusy}
          onNew={startNewDraft}
          onSelect={selectDraft}
          onDelete={(saved) => {
            const unsaved =
              saved.id === draftId && dirty
                ? " Your unsaved edits will also be discarded."
                : "";
            if (
              window.confirm(
                `Delete "${saved.title || "Untitled draft"}"? This cannot be undone.${unsaved}`,
              )
            ) {
              deleteMutation.mutate(saved.id);
            }
          }}
        />
      </aside>
    </div>
  );
}
