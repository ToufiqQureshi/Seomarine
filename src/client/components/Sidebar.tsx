import { Link, useLocation } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import {
  ArrowLeftRight,
  Check,
  ChevronsUpDown,
  CreditCard,
  LogOut,
  Settings,
} from "lucide-react";
import { organizationContextQueryOptions } from "@/client/features/team/organizationQueries";
import { switchOrganization } from "@/serverFunctions/organization";
import {
  accountNavGroup,
  connectNavGroup,
  getProjectNavGroups,
  type NavItem,
} from "@/client/navigation/items";
import { Badge } from "@/client/components/ui/badge";
import { ProjectSwitcher } from "@/client/features/projects/ProjectSwitcher";
import {
  SamChatListSkeleton,
  SamSidebarPanel,
} from "@/client/features/sam/SamSidebarPanel";
import { ThemePreferenceRadio } from "@/client/components/ThemePreferenceMenuItems";
import { getStandardErrorMessage } from "@/client/lib/error-messages";
import { signOutAndRedirect, useSession } from "@/lib/auth-client";
import { BILLING_ROUTE } from "@/shared/billing";
import {
  Sidebar as UiSidebar,
  SidebarContent,
  SidebarFooter as UiSidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  useSidebar,
} from "@/client/components/ui/sidebar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/client/components/ui/dropdown-menu";

// The active page is a raised card in the sidebar with its icon in the brand
// color; other rows stay quiet until hovered.
const navButtonClass =
  "h-9 rounded-lg text-sidebar-foreground/75 hover:bg-sidebar-accent hover:text-sidebar-foreground [&_svg]:text-sidebar-foreground/55 data-active:bg-card data-active:font-semibold data-active:text-foreground data-active:shadow-sm data-active:ring-1 data-active:ring-sidebar-border data-active:[&_svg]:text-primary";

function SidebarNavLink({
  item: { icon: Icon, label, badge, link },
  placeholder = false,
}: {
  item: NavItem;
  /** Show the row without its link, while its project is still unknown. */
  placeholder?: boolean;
}) {
  const { setOpenMobile } = useSidebar();
  const row = (isActive: boolean) => (
    <SidebarMenuButton
      render={<span />}
      isActive={isActive}
      aria-disabled={placeholder || undefined}
      className={navButtonClass}
    >
      <Icon className="size-4" />
      <span className="truncate">{label}</span>
      {badge ? (
        <Badge size="sm" className="ml-auto">
          {badge}
        </Badge>
      ) : null}
    </SidebarMenuButton>
  );
  return (
    <SidebarMenuItem>
      {placeholder ? (
        row(false)
      ) : (
        <Link
          {...link}
          activeOptions={{
            exact: false,
            includeSearch: false,
            ...link.activeOptions,
          }}
          onClick={() => setOpenMobile(false)}
        >
          {({ isActive }) => row(isActive)}
        </Link>
      )}
    </SidebarMenuItem>
  );
}

export function Sidebar({
  projectId,
  projectPending,
  ready,
}: {
  projectId: string | null;
  /** No project is known yet, but the projects list may still name one. */
  projectPending: boolean;
  /** The session is confirmed, so the sidebar's own data can load. */
  ready: boolean;
}) {
  // Until a project is known, show the project nav as rows without links, so
  // the sidebar has its full shape from the first paint instead of growing
  // once projects load. The placeholder rows never render these links.
  const navPlaceholder = projectId === null && projectPending;
  const navGroups =
    projectId !== null || navPlaceholder
      ? getProjectNavGroups(projectId ?? "")
      : [connectNavGroup];
  const { pathname } = useLocation();
  const { setOpenMobile } = useSidebar();
  // SAM is hidden from navigation: the chat route is only reachable by URL,
  // and while it is open its chat list takes the nav's place.
  const onSamRoute = pathname.includes("/sam");

  return (
    <UiSidebar collapsible="offcanvas">
      <SidebarHeader className="gap-3 px-3 pb-2 pt-4">
        <Link
          to="/"
          onClick={() => setOpenMobile(false)}
          className="flex items-center gap-2.5 rounded-lg px-1 text-sidebar-foreground"
        >
          <img src="/logo.svg" alt="" className="size-8" />
          <span className="flex flex-col leading-tight">
            <span className="text-base font-bold tracking-tight">
              Seomarine
            </span>
            <span className="text-xs text-sidebar-foreground/60">
              SEO + AI search
            </span>
          </span>
        </Link>
        <ProjectSwitcher
          activeProjectId={projectId}
          ready={ready}
          onCloseDrawer={() => setOpenMobile(false)}
        />
      </SidebarHeader>

      <SidebarContent>
        {onSamRoute && projectId ? (
          ready ? (
            <SamSidebarPanel
              projectId={projectId}
              onNavigate={() => setOpenMobile(false)}
            />
          ) : (
            <div className="px-2 py-1">
              <SamChatListSkeleton />
            </div>
          )
        ) : (
          <nav
            aria-label="Main navigation"
            aria-busy={navPlaceholder || undefined}
          >
            {navGroups.map((group) => (
              <SidebarGroup key={group.label} className="px-3 py-1.5">
                <SidebarGroupLabel className="h-7 px-1 font-semibold text-sidebar-foreground/60">
                  {group.label}
                </SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {group.items.map((item) => (
                      <SidebarNavLink
                        key={item.label}
                        item={item}
                        placeholder={navPlaceholder}
                      />
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            ))}
          </nav>
        )}
      </SidebarContent>
      <AccountFooter ready={ready} />
    </UiSidebar>
  );
}

function AccountFooter({ ready }: { ready: boolean }) {
  const { data: session } = useSession();
  const { setOpenMobile } = useSidebar();
  const email = session?.user?.email;
  const [isSwitching, setIsSwitching] = useState(false);
  const orgContextQuery = useQuery({
    ...organizationContextQueryOptions(),
    enabled: Boolean(email),
  });
  const organizations = orgContextQuery.data?.organizations ?? [];
  const activeOrganizationId = orgContextQuery.data?.organizationId;

  async function handleSwitchOrganization(organizationId: string) {
    if (isSwitching || organizationId === activeOrganizationId) return;
    setIsSwitching(true);
    try {
      await switchOrganization({ data: { organizationId } });
      window.location.assign("/");
    } catch (error) {
      toast.error(getStandardErrorMessage(error));
      setIsSwitching(false);
    }
  }

  return (
    <UiSidebarFooter className="gap-1 border-t border-sidebar-border px-3 pb-safe">
      <SidebarMenu>
        {accountNavGroup.items.map((item) => (
          <SidebarNavLink key={item.label} item={item} />
        ))}
        {email ? (
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <SidebarMenuButton
                    size="lg"
                    className="mt-1 h-11 rounded-lg hover:bg-sidebar-accent"
                    aria-label="Open account menu"
                  />
                }
              >
                <span
                  aria-hidden
                  className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold uppercase text-primary-foreground"
                >
                  {email.charAt(0)}
                </span>
                <span className="min-w-0 flex-1 truncate" data-ph-mask>
                  {email}
                </span>
                <ChevronsUpDown className="size-4 text-sidebar-foreground/55" />
              </DropdownMenuTrigger>
              <DropdownMenuContent side="top" className="w-56">
                {organizations.length > 1 ? (
                  <>
                    <DropdownMenuGroup>
                      <DropdownMenuLabel className="flex items-center gap-1.5">
                        <ArrowLeftRight className="size-3" />
                        Organization
                      </DropdownMenuLabel>
                      {organizations.map((organization) => (
                        <DropdownMenuItem
                          key={organization.organizationId}
                          disabled={isSwitching}
                          onClick={() =>
                            void handleSwitchOrganization(
                              organization.organizationId,
                            )
                          }
                        >
                          <span className="truncate">
                            {organization.organizationName}
                          </span>
                          {organization.organizationId ===
                          activeOrganizationId ? (
                            <Check className="ml-auto size-4" />
                          ) : null}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuGroup>
                    <DropdownMenuSeparator />
                  </>
                ) : null}
                <DropdownMenuItem
                  render={
                    <Link to="/settings" onClick={() => setOpenMobile(false)} />
                  }
                >
                  <Settings className="size-4" />
                  Settings
                </DropdownMenuItem>
                <DropdownMenuItem
                  render={
                    <Link
                      to={BILLING_ROUTE}
                      onClick={() => setOpenMobile(false)}
                    />
                  }
                >
                  <CreditCard className="size-4" />
                  Usage & credits
                </DropdownMenuItem>
                <DropdownMenuGroup>
                  <DropdownMenuLabel>Theme</DropdownMenuLabel>
                  <div className="px-1 pb-1">
                    <ThemePreferenceRadio />
                  </div>
                </DropdownMenuGroup>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => signOutAndRedirect()}
                >
                  <LogOut className="size-4" />
                  Sign out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        ) : ready ? (
          <SidebarNavLink
            item={{
              label: "Settings",
              icon: Settings,
              link: { to: "/settings" },
            }}
          />
        ) : (
          // The account row's slot while the session loads.
          <SidebarMenuItem>
            <SidebarMenuSkeleton showIcon />
          </SidebarMenuItem>
        )}
      </SidebarMenu>
    </UiSidebarFooter>
  );
}
