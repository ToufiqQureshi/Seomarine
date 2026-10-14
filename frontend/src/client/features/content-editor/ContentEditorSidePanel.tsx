import { useState } from "react";
import { DraftsPanel, ReadinessPanel, SearchPreview } from "./EditorPanels";
import { PublishToSite } from "@/client/features/site-publishing/PublishToSite";
import { WriterAssistant } from "./WriterAssistant";
import type { ContentDraftSummary } from "./contentEditorApi";
import type { ContentDraftInput, EditorMode, ScoreCheck } from "./scoring";

type EditorTab = "score" | "brief" | "outline" | "ai" | "publish";

export function ContentEditorSidePanel({
  projectId,
  draft,
  draftId,
  dirty,
  mode,
  seo,
  aeo,
  activeScore,
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
  activeScore: {
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
  const [tab, setTab] = useState<EditorTab>("score");
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
            onClick={() => setTab(item.id)}
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
          <ReadinessPanel
            mode={mode}
            seo={seo}
            aeo={aeo}
            activeScore={activeScore.score}
            checks={activeScore.checks}
            hasBrief={activeScore.hasBrief}
            blocksPublish={activeScore.blocksPublish}
            capReason={activeScore.capReason}
          />
        ) : null}
        {tab === "brief" ? (
          <section className="rounded-xl border bg-card p-4">
            <h2 className="font-medium">Search brief</h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              {activeScore.hasBrief
                ? "Brief terms, questions and competitor context are included in the score."
                : "No search brief is loaded. This editor currently scores checks that work without competitor data."}
            </p>
            {!activeScore.hasBrief ? (
              <p className="mt-3 text-xs text-muted-foreground">
                Term ranges, competitor depth and reader questions will appear
                here when a brief is available.
              </p>
            ) : null}
          </section>
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
              disabled={editorBusy}
              onUse={onUseSuggestion}
            />
          </section>
        ) : null}
        {tab === "publish" ? (
          <PublishToSite
            projectId={projectId}
            draftId={draftId}
            hasUnsavedChanges={dirty}
            blocksPublish={seo.blocksPublish}
          />
        ) : null}
      </div>
      <details className="group">
        <summary className="cursor-pointer list-none rounded-lg border bg-card px-4 py-3 text-sm font-medium marker:hidden">
          How it looks on Google
        </summary>
        <div className="pt-3">
          <SearchPreview draft={draft} />
        </div>
      </details>
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
