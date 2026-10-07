import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { AnalyticsPage } from "@/client/features/analytics/AnalyticsPage";

const searchSchema = z.object({
  days: z.union([z.literal(7), z.literal(30), z.literal(90)]).catch(30),
});

export const Route = createFileRoute("/_app/p/$projectId/analytics")({
  validateSearch: searchSchema,
  component: AnalyticsRoute,
});

function AnalyticsRoute() {
  const { projectId } = Route.useParams();
  const { days } = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <AnalyticsPage
      projectId={projectId}
      days={days}
      onDaysChange={(next) => void navigate({ search: { days: next } })}
    />
  );
}
