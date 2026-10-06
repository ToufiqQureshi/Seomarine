import { createFileRoute } from "@tanstack/react-router";
import { BrandingSettings } from "@/client/features/settings/BrandingSettings";

export const Route = createFileRoute("/_app/settings/branding")({
  component: BrandingSettings,
});
