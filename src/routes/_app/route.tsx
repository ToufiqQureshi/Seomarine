import { Outlet, createFileRoute, useParams } from "@tanstack/react-router";
import { useHostedAuthRouteGuard } from "@/client/features/auth/useHostedAuthRouteGuard";
import { FreePlanBanner } from "@/client/features/billing/FreePlanBanner";
import { SkeletonPage } from "@/client/components/SkeletonPresets";
import { AuthenticatedAppLayout } from "@/client/layout/AppShell";
import { useOnboardingRedirect } from "@/client/features/onboarding/useOnboardingRedirect";

export const Route = createFileRoute("/_app")({
  component: AppRouteLayout,
});

// One layout for app pages and project pages (/p/$projectId/...), so the
// drawer, setup modal and GSC nudge keep their state when the user moves
// between the two.
function AppRouteLayout() {
  const authGate = useHostedAuthRouteGuard();
  useOnboardingRedirect();
  const { projectId } = useParams({ strict: false });
  // The shell paints at once, and only the page waits for the session check,
  // so a full load shows the sidebar and a page skeleton instead of a spinner.
  const ready = authGate.canRenderAuthenticatedContent;

  return (
    <AuthenticatedAppLayout
      projectId={projectId}
      ready={ready}
      banner={
        ready && projectId && authGate.isHostedMode ? (
          <FreePlanBanner />
        ) : undefined
      }
    >
      {ready ? <Outlet /> : <SkeletonPage />}
    </AuthenticatedAppLayout>
  );
}
