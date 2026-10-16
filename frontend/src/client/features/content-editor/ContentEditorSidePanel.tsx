import { BriefPanel } from "./BriefPanel";
import { briefForWriter } from "./brief-view";
import { DraftsPanel } from "./EditorPanels";
import { LinkSuggestionsCard, PublishTabPanel } from "./FreeSeoPublishCards";
import { ScorePanel } from "./ScorePanel";
import { PublishToSite } from "@/client/features/site-publishing/PublishToSite";
import { WriterAssistant } from "./WriterAssistant";
import { primaryTargetTopic } from "./target-topics";
import type { ContentDraftSummary, SearchBrief } from "./contentEditorApi";
import type { ContentDraftInput, EditorMode, ScoreCheck } from "./scoring";

export type EditorTab = "score" | "brief" | "outline" | "ai" | "publish";

export function ContentEditorSidePanel({
  projectId,
  draft,
  draftId,
  dirty,
  mode,
  onModeChange,
  brief,
  onBrief,
  tab,
  onTabChange,
  seo,
  aeo,
  editorBusy,
  drafts,
  draftsLoading,
  draftsError,
  onNewDraft,
  onSelectDraft,
  onDeleteDraft,
  onUseSuggestion,
}: {
  projectId: string;
  draft: ContentDraftInput;
  draftId?: string;
  dirty: boolean;
  mode: EditorMode;
  onModeChange: (mode: EditorMode) => void;
  brief: SearchBrief | null;
  onBrief: (brief: SearchBrief) => void;
  tab: EditorTab;
  onTabChange: (tab: EditorTab) => void;
  seo: {
    score: number;
    checks: ScoreCheck[];
    hasBrief: boolean;
    blocksPublish: boolean;
    capReason?: string;
  };
  aeo: {
    score: number;
    checks: ScoreCheck[];
    hasBrief: boolean;
    blocksPublish: boolean;
    capReason?: string;
  };
  editorBusy: boolean;
  drafts: ContentDraftSummary[];
  draftsLoading: boolean;
  draftsError: boolean;
  onNewDraft: () => void;
  onSelectDraft: (draft: ContentDraftSummary) => void;
  onDeleteDraft: (draft: ContentDraftSummary) => void;
  onUseSuggestion: (content: string) => void;
}) {
  const outline = draft.content.split("\n").flatMap((line) => {
    const match = line.trim().match(/^(#{1,3})\s+(.+)$/u);
    return match ? [{ level: match[1].length, text: match[2] }] : [];
  });
  const tabs: Array<{ id: EditorTab; label: string }> = [
    { id: "score", label: "Score" },
    { id: "brief", label: "Brief" },
    { id: "outline", label: "Outline" },
    { id: "ai", label: "AI help" },
    { id: "publish", label: "Publish" },
  ];

  return (
    <aside className="min-w-0 space-y-3">
      <div
        role="tablist"
        aria-label="Editor tools"
        className="grid grid-cols-5 rounded-lg border bg-card p-1"
      >
        {tabs.map((item) => (
          <button
            key={item.id}
            id={`editor-tab-${item.id}`}
            type="button"
            role="tab"
            aria-selected={tab === item.id}
            aria-controls="editor-tab-panel"
            onClick={() => onTabChange(item.id)}
            className={`min-h-10 rounded-md px-2 text-xs font-medium sm:text-sm ${tab === item.id ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground"}`}
          >
            {item.label}
          </button>
        ))}
      </div>
      <div
        id="editor-tab-panel"
        role="tabpanel"
        aria-labelledby={`editor-tab-${tab}`}
        className="min-w-0 space-y-3"
      >
        {tab === "score" ? (
          <ScorePanel
            mode={mode}
            onModeChange={onModeChange}
            seo={seo}
            aeo={aeo}
          />
        ) : null}
        {tab === "brief" ? (
          <BriefPanel
            projectId={projectId}
            keyword={primaryTargetTopic(draft.targetKeyword)}
            content={draft.content}
            brief={brief}
            onBrief={onBrief}
          />
        ) : null}
        {tab === "outline" ? (
          <section className="rounded-xl border bg-card p-4">
            <h2 className="font-medium">Article outline</h2>
            {outline.length ? (
              <ol className="mt-3 space-y-2 text-sm">
                {outline.map((item, index) => (
                  <li
                    key={`${item.level}-${item.text}-${index}`}
                    className={
                      item.level > 1
                        ? "pl-4 text-muted-foreground"
                        : "font-medium"
                    }
                  >
                    {item.text}
                  </li>
                ))}
              </ol>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">
                Add H2 or H3 headings to build an outline.
              </p>
            )}
          </section>
        ) : null}
        {tab === "ai" ? (
          <section className="space-y-3 rounded-xl border bg-card p-4">
            <div>
              <h2 className="font-medium">AI writing help</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Optional; AI actions use workspace credits.
              </p>
            </div>
            <WriterAssistant
              projectId={projectId}
              draft={draft}
              mode={mode}
              brief={briefForWriter(brief)}
              disabled={editorBusy}
              onUse={onUseSuggestion}
            />
          </section>
        ) : null}
        {tab === "publish" ? (
          <PublishTabPanel
            projectId={projectId}
            draftId={draftId}
            draft={draft}
            editorBusy={editorBusy}
            publishToSite={(schema) => (
              <PublishToSite
                projectId={projectId}
                draftId={draftId}
                hasUnsavedChanges={dirty}
                blocksPublish={seo.blocksPublish}
                targetKeyword={primaryTargetTopic(draft.targetKeyword)}
                schema={schema}
              />
            )}
          />
        ) : null}
      </div>
      <LinkSuggestionsCard
        projectId={projectId}
        draft={draft}
        isBusy={editorBusy}
      />
      <details className="group">
        <summary className="cursor-pointer list-none rounded-lg border bg-card px-4 py-3 text-sm font-medium marker:hidden">
          Your drafts
          <span className="ml-2 text-xs font-normal text-muted-foreground">
            {drafts.length}
          </span>
        </summary>
        <div className="pt-3">
          <DraftsPanel
            drafts={drafts}
            isLoading={draftsLoading}
            hasError={draftsError}
            currentDraftId={draftId}
            isBusy={editorBusy}
            onNew={onNewDraft}
            onSelect={onSelectDraft}
            onDelete={onDeleteDraft}
          />
        </div>
      </details>
    </aside>
  );
}
