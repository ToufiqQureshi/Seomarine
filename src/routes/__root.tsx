/// <reference types="vite/client" />
import { Outlet, createRootRoute } from "@tanstack/react-router";
import { TanStackRouterDevtoolsPanel } from "@tanstack/react-router-devtools";
import { TanStackDevtools } from "@tanstack/react-devtools";
import { QueryClientProvider } from "@tanstack/react-query";
import * as React from "react";
import { DefaultCatchBoundary } from "@/client/components/DefaultCatchBoundary";
import { captureGoogleLinkError } from "@/client/features/integrations/googleLinkError";
import { ExportToSheetsModal } from "@/client/components/table/ExportToSheetsModal";
import {
  identifyAnalyticsUser,
  resetAnalyticsUser,
  startAnalyticsCapture,
  stopAnalyticsCapture,
} from "@/client/lib/posthog";
import { NotFound } from "@/client/components/NotFound";
import { TooltipProvider } from "@/client/components/ui/tooltip";
import { useSession } from "@/lib/auth-client";
import { isHostedClientAuthMode } from "@/lib/auth-mode";
import { Toaster } from "sonner";
import { queryClient } from "@/client/tanstack-db";
import { getActiveOrganizationId } from "@/lib/auth-session";

// Capture Google link error params before the router starts — a route loader
// redirect would otherwise replace the URL and lose them. See googleLinkError.ts.
captureGoogleLinkError();

export const Route = createRootRoute({
  component: AppLayout,
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
});

function PostHogBootstrap() {
  const isHostedMode = isHostedClientAuthMode();
  const { data: session, isPending: isSessionPending } = useSession();
  const userId = session?.user?.id ?? null;
  const optedOut = session?.user?.analyticsOptedOut === true;
  const organizationId = getActiveOrganizationId(session);
  const previousUserIdRef = React.useRef<string | null>(null);

  React.useEffect(() => {
    if (!isHostedMode || isSessionPending) {
      return;
    }

    if (userId && !optedOut) {
      startAnalyticsCapture();
      identifyAnalyticsUser({ userId, organizationId });
      previousUserIdRef.current = userId;
    } else if (userId && optedOut) {
      stopAnalyticsCapture();
    } else if (previousUserIdRef.current) {
      previousUserIdRef.current = null;
      resetAnalyticsUser();
    }
  }, [isHostedMode, isSessionPending, optedOut, organizationId, userId]);

  return null;
}

function AppLayout() {
  const showDevtools =
    import.meta.env.DEV && import.meta.env.VITE_SHOW_DEVTOOLS !== "false";

  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <PostHogBootstrap />
        <Outlet />
        <ExportToSheetsModal />
        <Toaster position="bottom-right" mobileOffset={{ bottom: 100 }} />
        {showDevtools ? (
          <TanStackDevtools
            config={{ position: "bottom-right" }}
            eventBusConfig={{ connectToServerBus: true }}
            plugins={[
              {
                name: "TanStack Router",
                render: <TanStackRouterDevtoolsPanel />,
                defaultOpen: true,
              },
            ]}
          />
        ) : null}
      </TooltipProvider>
    </QueryClientProvider>
  );
}
