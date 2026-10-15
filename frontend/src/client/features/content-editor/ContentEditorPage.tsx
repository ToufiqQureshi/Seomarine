import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Monitor, Save, Send } from "lucide-react";
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
import { WriterReminder } from "./EditorPanels";
import { ArticleComposer } from "./ArticleComposer";
import {
  ContentEditorSidePanel,
  type EditorTab,
} from "./ContentEditorSidePanel";
import { isArticleContentWithinLimit } from "./content-limits";

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
const slugFromTitle = (title: string) =>
  title
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/gu, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/gu, "-")
    .replace(/^-|-$/gu, "")
    .slice(0, 300);
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
  const [tab, setTab] = useState<EditorTab>("score");
  const [previewOpen, setPreviewOpen] = useState(false);
  const [draft, setDraft] = useState<ContentDraftInput>(blankDraft);
  const [draftId, setDraftId] = useState<string>();
  const [savedSnapshot, setSavedSnapshot] = useState(blankDraft);
  const [initialized, setInitialized] = useState(false);
  const [imageUploadBusy, setImageUploadBusy] = useState(false);
  const [scoredDraft, setScoredDraft] = useState(draft);
  const [slugEdited, setSlugEdited] = useState(false);
  const dirty = JSON.stringify(draft) !== JSON.stringify(savedSnapshot);
  useEffect(() => {
    const timer = window.setTimeout(() => setScoredDraft(draft), 300);
    return () => window.clearTimeout(timer);
  }, [draft]);
  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
  const seo = useMemo(() => scoreContent("seo", scoredDraft), [scoredDraft]);
  const aeo = useMemo(() => scoreContent("aeo", scoredDraft), [scoredDraft]);
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
      setSlugEdited(snapshot.slug !== slugFromTitle(snapshot.title));
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
      setSlugEdited(snapshot.slug !== slugFromTitle(snapshot.title));
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
    setSlugEdited(false);
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

  const update = (field: keyof ContentDraftInput, value: string) => {
    if (field === "slug") setSlugEdited(true);
    setDraft((current) => {
      if (field === "title" && !slugEdited) {
        const slug = slugFromTitle(value);
        return { ...current, title: value, slug };
      }
      return { ...current, [field]: value };
    });
  };
  const words = wordCount(draft.content);
  const readMinutes = Math.ceil(words / 200);

  return (
    <div className="space-y-5">
      <div className="sticky top-0 z-20 -mx-1 flex flex-wrap items-center gap-3 rounded-xl border bg-background/95 px-4 py-3 backdrop-blur">
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">
            {draft.title || "Untitled draft"}
          </p>
          <p className="text-xs text-muted-foreground">
            {words.toLocaleString()} words ·{" "}
            {words > 0 ? `about ${readMinutes} min read · ` : ""}
            {dirty ? "Unsaved changes" : "All changes saved"}
          </p>
        </div>
        <Button variant="outline" onClick={() => setPreviewOpen(true)}>
          <Monitor aria-hidden />
          Preview as on website
        </Button>
        <Button
          variant="outline"
          onClick={() => saveMutation.mutate()}
          disabled={
            editorBusy ||
            !isArticleContentWithinLimit(draft.content) ||
            (!draft.title.trim() && !draft.content.trim())
          }
        >
          <Save aria-hidden />
          {saveMutation.isPending ? "Saving…" : "Save draft"}
        </Button>
        <Button onClick={() => setTab("publish")}>
          <Send aria-hidden />
          Publish
        </Button>
      </div>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
        <section className="min-w-0 space-y-5">
          <WriterReminder key={projectId} projectId={projectId} />
          <ArticleComposer
            draft={draft}
            projectId={projectId}
            onTopicLink={
              draft.targetKeyword.trim()
                ? `/p/${projectId}/keywords?q=${encodeURIComponent(draft.targetKeyword.trim())}`
                : undefined
            }
            disabled={editorBusy}
            onImageUploadStateChange={setImageUploadBusy}
            onChange={update}
            previewOpen={previewOpen}
            onPreviewOpenChange={setPreviewOpen}
          />
        </section>

        <ContentEditorSidePanel
          projectId={projectId}
          draft={draft}
          draftId={draftId}
          dirty={dirty}
          mode={mode}
          onModeChange={setMode}
          tab={tab}
          onTabChange={setTab}
          seo={seo}
          aeo={aeo}
          activeScore={activeScore}
          editorBusy={editorBusy}
          drafts={draftsQuery.data ?? []}
          draftsLoading={draftsQuery.isPending}
          draftsError={draftsQuery.isError}
          onNewDraft={startNewDraft}
          onSelectDraft={selectDraft}
          onDeleteDraft={(saved) => {
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
          onUseSuggestion={(content) =>
            confirmSuggestionReplacement(draft.content) &&
            update("content", content)
          }
        />
      </div>
    </div>
  );
}
