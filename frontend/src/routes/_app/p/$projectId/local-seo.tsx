import { createFileRoute } from "@tanstack/react-router";
import { LocalSEOPage } from "@/client/features/local-seo/LocalSEOPage";

export const Route = createFileRoute("/_app/p/$projectId/local-seo")({
  component: LocalSEORoute,
});

function LocalSEORoute() {
  const { projectId } = Route.useParams();
  return <LocalSEOPage projectId={projectId} />;
}
