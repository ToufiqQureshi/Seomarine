import {
  Bookmark,
  Bot,
  Brain,
  ChartNoAxesCombined,
  CircleHelp,
  ClipboardCheck,
  CreditCard,
  FileText,
  Globe,
  LayoutDashboard,
  Link2,
  MessageSquare,
  Search,
  Sparkles,
  TrendingUp,
} from "lucide-react";
import { linkOptions, type LinkOptions } from "@tanstack/react-router";
import type { ComponentType } from "react";
import { GoogleGlyphMuted } from "@/client/features/gsc/GoogleGlyph";

// Labels match each page's own title, so the header breadcrumb and the page
// heading agree.
const projectNavItems = [
  {
    to: "/p/$projectId" as const,
    label: "Dashboard",
    icon: LayoutDashboard,
    // Without exact matching, the index path is a prefix of every project
    // route and the Dashboard item would render active everywhere.
    activeOptions: { exact: true, includeSearch: false },
  },
  {
    to: "/p/$projectId/analytics" as const,
    label: "Analytics",
    icon: ChartNoAxesCombined,
    badge: "New",
  },
  {
    to: "/p/$projectId/search-performance" as const,
    label: "Search Performance",
    icon: GoogleGlyphMuted,
  },
  {
    to: "/p/$projectId/brand-lookup" as const,
    label: "Brand Lookup",
    icon: Sparkles,
  },
  {
    to: "/p/$projectId/prompt-explorer" as const,
    label: "Prompt Explorer",
    icon: MessageSquare,
  },
  {
    to: "/p/$projectId/rank-tracking" as const,
    label: "Rank Tracking",
    icon: TrendingUp,
  },
  {
    to: "/p/$projectId/audit" as const,
    label: "Site Audit",
    icon: ClipboardCheck,
  },
  {
    to: "/p/$projectId/saved" as const,
    label: "Saved Keywords",
    icon: Bookmark,
  },
  {
    to: "/p/$projectId/keywords" as const,
    label: "Keyword Research",
    icon: Search,
  },
  {
    to: "/p/$projectId/domain" as const,
    label: "Domain Overview",
    icon: Globe,
  },
  {
    to: "/p/$projectId/backlinks" as const,
    label: "Backlinks",
    icon: Link2,
  },
  {
    to: "/p/$projectId/reports" as const,
    label: "Reports",
    icon: FileText,
  },
  {
    to: "/p/$projectId/context" as const,
    label: "Context",
    icon: Brain,
  },
] as const;

type ProjectNavPath = (typeof projectNavItems)[number]["to"];

export type NavItem = {
  label: string;
  icon: ComponentType<{ className?: string }>;
  /** A short tag after the label, such as "New". */
  badge?: string;
  link: LinkOptions;
};

type NavGroup = { label: string; items: NavItem[] };

// Project-independent. Rendered inside the project "Reports & AI" group when a
// project is selected, and on its own (connectNavGroup) when none is.
const aiNavItem: NavItem = {
  label: "Agent setup",
  icon: Bot,
  link: linkOptions({ to: "/ai" }),
};

// Shown only when no project is selected; with a project, Agent setup lives in
// the "Reports & AI" group below.
export const connectNavGroup: NavGroup = {
  label: "AI",
  items: [aiNavItem],
};

// Pinned to the sidebar footer, above the account menu.
export const accountNavGroup: NavGroup = {
  label: "Account",
  items: [
    {
      label: "Billing",
      icon: CreditCard,
      link: linkOptions({ to: "/billing/plan" }),
    },
    { label: "Help", icon: CircleHelp, link: linkOptions({ to: "/support" }) },
  ],
};

/** One project page's link, label and icon, shared by the nav and shortcuts. */
export function getProjectNavItem(
  projectId: string,
  path: ProjectNavPath,
): NavItem {
  const { label, icon, ...item } = projectNavItems.find(
    (navItem) => navItem.to === path,
  )!;
  return {
    label,
    icon,
    badge: "badge" in item ? item.badge : undefined,
    link: linkOptions({
      to: item.to,
      activeOptions: "activeOptions" in item ? item.activeOptions : undefined,
      params: { projectId },
      search: {},
    }),
  };
}

// Grouped by the job the user came to do: traffic and AI presence first (the
// product's focus), then looking after their own site, then researching
// anything, then sharing and automating.
export function getProjectNavGroups(projectId: string): NavGroup[] {
  const item = (path: ProjectNavPath) => getProjectNavItem(projectId, path);

  return [
    {
      label: "Home",
      items: [item("/p/$projectId")],
    },
    {
      label: "Traffic & AI visibility",
      items: [
        item("/p/$projectId/analytics"),
        item("/p/$projectId/search-performance"),
        item("/p/$projectId/brand-lookup"),
        item("/p/$projectId/prompt-explorer"),
      ],
    },
    {
      label: "Your site",
      items: [
        item("/p/$projectId/rank-tracking"),
        item("/p/$projectId/audit"),
        item("/p/$projectId/saved"),
      ],
    },
    {
      label: "Research",
      items: [
        item("/p/$projectId/keywords"),
        item("/p/$projectId/domain"),
        item("/p/$projectId/backlinks"),
      ],
    },
    {
      label: "Reports & AI",
      items: [
        item("/p/$projectId/reports"),
        item("/p/$projectId/context"),
        aiNavItem,
      ],
    },
  ];
}

export const dataforseoHelpLinkOptions = linkOptions({
  to: "/help/dataforseo-api-key",
});
