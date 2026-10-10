import * as React from "react";
import { projectsQueryOptions } from "@/client/features/projects/projectQueries";
import { Link, useLocation, useMatchRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  MissingSeoSetupModal,
  SeoApiStatusBanners,
} from "@/client/layout/AppShellParts";
import { GscReEngagementModal } from "@/client/features/gsc/GscReEngagementModal";
import { Sidebar } from "@/client/components/Sidebar";
import { BILLING_ROUTE } from "@/shared/billing";
import { getSeoApiKeyStatus } from "@/client/lib/appConfigApi";
import { getLastProjectId } from "@/client/lib/active-project";
import {
  accountNavGroup,
  connectNavGroup,
  dataforseoHelpLinkOptions,
  getProjectNavGroups,
} from "@/client/navigation/items";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/client/components/ui/sidebar";

export function AuthenticatedAppLayout({
  children,
  projectId,
  ready,
  banner,
}: {
  children: React.ReactNode;
  projectId?: string;
  /** The session is confirmed. Until then the shell renders but loads nothing. */
  ready: boolean;
  banner?: React.ReactNode;
}) {
  const location = useLocation();
  const [showMissingSeoApiKeyModal, setShowMissingSeoApiKeyModal] =
    React.useState(false);
  // On non-project pages (e.g. /settings) there's no projectId in the URL, so
  // derive one for the nav/switcher: prefer the last-visited project, else the
  // most recent. The whole app tree is client-only (see root ClientOnly), so we
  // can read localStorage synchronously during render — this lets the sidebar
  // show the full project nav on the very first paint instead of briefly
  // flashing only the always-visible Connect group while projects load. It is
  // read on every render, not once: the shell stays mounted across project
  // pages, which update the remembered project as the user moves between them.
  const projectsQuery = useQuery({
    ...projectsQueryOptions(),
    enabled: ready && !projectId,
  });
  const rememberedProjectId = getLastProjectId();
  const fallbackProjects = projectsQuery.data ?? [];
  const fallbackProjectId =
    fallbackProjects.find((project) => project.id === rememberedProjectId)
      ?.id ??
    fallbackProjects[0]?.id ??
    null;
  // Once the projects list loads, fallbackProjectId is the validated choice
  // (remembered-if-valid, else most recent). Before it loads, fall back to the
  // remembered id so the project nav renders immediately; a stale id here only
  // builds links that self-correct via the route guard once data arrives.
  const sidebarProjectId =
    projectId ?? fallbackProjectId ?? rememberedProjectId;
  // No project to show yet, but the list that may name one is still loading
  // (a first visit to a page without a project in the URL).
  const sidebarProjectPending =
    sidebarProjectId === null && projectsQuery.isPending;
  // The setup guide is where the modal and banners send the user, so it shows
  // neither: a banner there would link to the page the user is already on.
  const shouldCheckSeoApiKeyStatus =
    location.pathname !== BILLING_ROUTE &&
    location.pathname !== dataforseoHelpLinkOptions.to;
  const seoApiKeyStatusQuery = useQuery({
    queryKey: ["seoApiKeyStatus"],
    queryFn: () => getSeoApiKeyStatus(),
    enabled: ready && shouldCheckSeoApiKeyStatus,
  });
  const isSeoApiKeyConfigured = shouldCheckSeoApiKeyStatus
    ? (seoApiKeyStatusQuery.data?.configured ?? null)
    : null;
  const seoApiKeyStatusError =
    shouldCheckSeoApiKeyStatus && seoApiKeyStatusQuery.isError;

  React.useEffect(() => {
    if (!shouldCheckSeoApiKeyStatus) {
      setShowMissingSeoApiKeyModal(false);
      return;
    }

    if (seoApiKeyStatusQuery.isError) {
      setShowMissingSeoApiKeyModal(false);
      return;
    }

    if (!seoApiKeyStatusQuery.isSuccess) return;
    setShowMissingSeoApiKeyModal(!seoApiKeyStatusQuery.data.configured);
  }, [
    location.pathname,
    seoApiKeyStatusQuery.data,
    seoApiKeyStatusQuery.isError,
    seoApiKeyStatusQuery.isSuccess,
    shouldCheckSeoApiKeyStatus,
  ]);

  const shouldShowSeoApiWarning =
    !seoApiKeyStatusError &&
    isSeoApiKeyConfigured === false &&
    !showMissingSeoApiKeyModal;

  return (
    <SidebarProvider className="h-[100dvh] min-h-0 overflow-hidden">
      <Sidebar
        projectId={sidebarProjectId}
        projectPending={sidebarProjectPending}
        ready={ready}
      />
      <SidebarInset className="min-h-0 overflow-hidden">
        <AppHeader projectId={sidebarProjectId} />
        <SeoApiStatusBanners
          shouldShowSeoApiWarning={shouldShowSeoApiWarning}
          seoApiKeyStatusError={seoApiKeyStatusError}
        />
        {banner}
        <div className="min-h-0 flex-1 overflow-auto">{children}</div>
      </SidebarInset>

      {showMissingSeoApiKeyModal ? (
        <MissingSeoSetupModal
          onClose={() => setShowMissingSeoApiKeyModal(false)}
        />
      ) : null}

      {ready ? (
        <GscReEngagementModal
          projectId={sidebarProjectId}
          suppressed={showMissingSeoApiKeyModal}
        />
      ) : null}
    </SidebarProvider>
  );
}

// The bar above every page: the sidebar toggle, the brand on small screens,
// and where the current page sits in the navigation.
function AppHeader({ projectId }: { projectId: string | null }) {
  const matchRoute = useMatchRoute();
  const groups = [
    ...(projectId === null
      ? [connectNavGroup]
      : getProjectNavGroups(projectId)),
    accountNavGroup,
  ];
  const current = groups
    .flatMap((group) => group.items.map((item) => ({ group, item })))
    .find(({ item }) =>
      matchRoute({
        ...item.link,
        fuzzy: !item.link.activeOptions?.exact,
      }),
    );

  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-card px-3 md:px-4">
      <SidebarTrigger aria-label="Toggle sidebar" />
      <Link
        to="/"
        className="flex items-center gap-2 font-bold tracking-tight text-foreground md:hidden"
      >
        <img src="/logo.svg" alt="" className="size-6" />
        Seomarine
      </Link>
      {current ? (
        <nav aria-label="Breadcrumb" className="hidden min-w-0 md:block">
          <ol className="flex items-center gap-1.5 text-sm">
            <li className="text-muted-foreground">{current.group.label}</li>
            <li aria-hidden className="text-muted-foreground">
              /
            </li>
            <li aria-current="page" className="truncate font-medium">
              {current.item.label}
            </li>
          </ol>
        </nav>
      ) : null}
    </header>
  );
}
