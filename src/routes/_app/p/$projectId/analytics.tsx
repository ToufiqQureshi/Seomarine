import { createFileRoute } from "@tanstack/react-router";
import { ChartNoAxesCombined } from "lucide-react";
import { EmptyState } from "@/client/components/EmptyState";
import { ProjectPageHeader } from "@/client/features/projects/ProjectPageHeader";

export const Route = createFileRoute("/_app/p/$projectId/analytics")({
  component: AnalyticsRoute,
});

// Placeholder until the analytics dashboard (served by the Go API) lands.
function AnalyticsRoute() {
  const { projectId } = Route.useParams();
  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <ProjectPageHeader projectId={projectId} title="Analytics" />
        <EmptyState
          variant="card"
          size="lg"
          icon={ChartNoAxesCombined}
          title="Analytics is on its way"
          description="Soon you will see who visits your site, which pages they read, and how many people arrive from ChatGPT, Perplexity, Gemini, Claude and Copilot."
        />
      </div>
    </div>
  );
}
