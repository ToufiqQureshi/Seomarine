import { Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { getProjectNavItem } from "@/client/navigation/items";

// The four jobs most people open Seomarine for, each with a plain-language
// line on what the page answers.
const SHORTCUTS = [
  {
    path: "/p/$projectId/analytics",
    description: "Who visits, and how many come from AI chats",
  },
  {
    path: "/p/$projectId/brand-lookup",
    description: "Where AI answers mention your brand",
  },
  {
    path: "/p/$projectId/rank-tracking",
    description: "Your Google positions, checked on a schedule",
  },
  {
    path: "/p/$projectId/audit",
    description: "Problems on your pages, with steps to fix them",
  },
] as const;

export function DashboardShortcuts({ projectId }: { projectId: string }) {
  return (
    <nav aria-label="Shortcuts">
      <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {SHORTCUTS.map(({ path, description }) => {
          const {
            label,
            icon: Icon,
            link,
          } = getProjectNavItem(projectId, path);
          return (
            <li key={path}>
              <Link
                {...link}
                className="group flex h-full items-start gap-3 rounded-xl border border-border bg-card p-4 shadow-xs transition-colors hover:border-primary/40"
              >
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <Icon className="size-4" />
                </span>
                <span className="min-w-0 flex-1 space-y-0.5">
                  <span className="flex items-center gap-1 text-sm font-semibold">
                    {label}
                    <ArrowRight
                      aria-hidden
                      className="size-3.5 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                    />
                  </span>
                  <span className="block text-sm text-muted-foreground">
                    {description}
                  </span>
                </span>
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
