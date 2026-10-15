import { createFileRoute } from "@tanstack/react-router";
import { PageHeader } from "@/client/components/PageHeader";
import { ContentEditorPage } from "@/client/features/content-editor/ContentEditorPage";
import { EditorSessionProvider } from "@/client/features/content-editor/editor-session";

export const Route = createFileRoute("/_app/p/$projectId/content-editor")({
  component: ContentEditorRoute,
});

function ContentEditorRoute() {
  const { projectId } = Route.useParams();
  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-[1500px] space-y-5">
        <PageHeader
          title="Content Editor"
          description="Write for people first. Switch between search visibility and answer readiness as you shape your draft."
        />
        <EditorSessionProvider>
          <ContentEditorPage key={projectId} projectId={projectId} />
        </EditorSessionProvider>
      </div>
    </div>
  );
}
