import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { sortBy } from "remeda";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/client/components/ui/button";
import {
  contentEditorQueryKeys,
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
  WriterReminder,
  SearchPreview,
  WritingModeToggle,
} from "./EditorPanels";
import { ArticleComposer } from "./ArticleComposer";
import { WriterAssistant } from "./WriterAssistant";

const blankDraft: ContentDraftInput = {
  title: "",
  targetKeyword: "",
  slug: "",
  metaDescription: "",
  content: "",
};

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
    queryKey: contentEditorQueryKeys.drafts(projectId),
    queryFn: () => listContentDrafts({ data: { projectId } }),
  });
  const [mode, setMode] = useState<EditorMode>("seo");
  const [draft, setDraft] = useState<ContentDraftInput>(blankDraft);
  const [draftId, setDraftId] = useState<string>();
  const [savedSnapshot, setSavedSnapshot] = useState(blankDraft);
  const [initialized, setInitialized] = useState(false);
  const [imageUploadBusy, setImageUploadBusy] = useState(false);
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
    onSuccess: async (saved) => {
      const snapshot = asInput(saved);
      setDraftId(saved.id);
      setDraft(snapshot);
      setSavedSnapshot(snapshot);
      await queryClient.cancelQueries({
        queryKey: contentEditorQueryKeys.drafts(projectId),
        exact: true,
      });
      queryClient.setQueryData(
        contentEditorQueryKeys.draft(projectId, saved.id),
        saved,
      );
      const draftListKey = contentEditorQueryKeys.drafts(projectId);
      const currentDrafts =
        queryClient.getQueryData<ContentDraftSummary[]>(draftListKey);
      if (currentDrafts) {
        const summary: ContentDraftSummary = {
          id: saved.id,
          projectId: saved.projectId,
          title: saved.title,
          targetKeyword: saved.targetKeyword,
          updatedAt: saved.updatedAt,
        };
        queryClient.setQueryData(
          draftListKey,
          sortBy(
            [summary, ...currentDrafts.filter((item) => item.id !== saved.id)],
            [(item) => item.updatedAt, "desc"],
          ),
        );
      } else {
        void queryClient.invalidateQueries({
          queryKey: draftListKey,
          exact: true,
        });
      }
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
      await queryClient.cancelQueries({
        queryKey: contentEditorQueryKeys.drafts(projectId),
        exact: true,
      });
      if (id === draftId) {
        setDraftId(undefined);
        setDraft(blankDraft);
        setSavedSnapshot(blankDraft);
      }
      queryClient.removeQueries({
        queryKey: contentEditorQueryKeys.draft(projectId, id),
        exact: true,
      });
      const draftListKey = contentEditorQueryKeys.drafts(projectId);
      const currentDrafts =
        queryClient.getQueryData<ContentDraftSummary[]>(draftListKey);
      if (currentDrafts) {
        queryClient.setQueryData(
          draftListKey,
          currentDrafts.filter((item) => item.id !== id),
        );
      } else {
        void queryClient.invalidateQueries({
          queryKey: draftListKey,
          exact: true,
        });
      }
      toast.success("Draft deleted");
    },
    onError: (error) =>
      toast.error(
        error instanceof Error ? error.message : "Could not delete this draft.",
      ),
  });

  const openMutation = useMutation({
    mutationFn: (id: string) =>
      queryClient.fetchQuery({
        queryKey: contentEditorQueryKeys.draft(projectId, id),
        queryFn: () => getContentDraft({ data: { projectId, draftId: id } }),
      }),
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
    deleteMutation.isPending ||
    imageUploadBusy;

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

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_350px]">
      <section className="min-w-0 space-y-5">
        <WriterReminder key={projectId} projectId={projectId} />
        <WritingModeToggle mode={mode} onChange={setMode} />
        <ArticleComposer
          draft={draft}
          projectId={projectId}
          disabled={editorBusy}
          onImageUploadStateChange={setImageUploadBusy}
          onChange={update}
        />
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-card px-4 py-3">
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
      </section>

      <aside className="space-y-3">
        <ReadinessPanel
          mode={mode}
          seo={seo}
          aeo={aeo}
          activeScore={activeScore.score}
          checks={activeScore.checks}
        />
        <details className="group">
          <summary className="cursor-pointer list-none rounded-lg border bg-card px-4 py-3 text-sm font-medium marker:hidden">
            AI writing assistant
            <span className="ml-2 text-xs font-normal text-muted-foreground">
              Optional · uses credits
            </span>
          </summary>
          <div className="pt-3">
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
          </div>
        </details>
        <details className="group">
          <summary className="cursor-pointer list-none rounded-lg border bg-card px-4 py-3 text-sm font-medium marker:hidden">
            Search preview
          </summary>
          <div className="pt-3">
            <SearchPreview draft={draft} />
          </div>
        </details>
        <details className="group">
          <summary className="cursor-pointer list-none rounded-lg border bg-card px-4 py-3 text-sm font-medium marker:hidden">
            Your drafts
            <span className="ml-2 text-xs font-normal text-muted-foreground">
              {draftsQuery.data?.length ?? 0}
            </span>
          </summary>
          <div className="pt-3">
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
          </div>
        </details>
      </aside>
    </div>
  );
}
