import { createFileRoute } from "@tanstack/react-router";
import { TeamSettings } from "@/client/features/team/TeamSettings";

export const Route = createFileRoute("/_app/settings/organization")({
  component: TeamSettings,
});
