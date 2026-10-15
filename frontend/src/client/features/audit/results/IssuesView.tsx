import { useMemo, useState, type ReactNode } from "react";
import { ChevronRight } from "lucide-react";
import { sort } from "remeda";
import {
  getIssueDescriptor,
  ISSUE_SEVERITY_ORDER,
  type IssueSeverity,
} from "@/shared/audit-issues";
import type { AuditResultsData } from "@/client/features/audit/results/types";
import { SeverityBadge } from "@/client/features/audit/shared";
import { EmptyState } from "@/client/components/EmptyState";
import { Button } from "@/client/components/ui/button";
import { toast } from "sonner";
import { getOrphanLinkCandidates } from "@/client/features/audit/orphanLinkCandidates";

type AuditIssueRow = AuditResultsData["issues"][number];

const MAX_RENDERED_URLS = 100;

const SEVERITY_RULE: Record<IssueSeverity, string> = {
  critical: "border-l-destructive/60",
  warning: "border-l-warning/60",
  info: "border-l-info/60",
};

const SEVERITY_LABEL: Record<IssueSeverity, string> = {
  critical: "Critical",
  warning: "Warning",
  info: "Info",
};

interface IssueGroup {
  issueType: string;
  severity: IssueSeverity;
  title: string;
  explanation: string;
  howToFix: string;
  issues: AuditIssueRow[];
}

export function resolveIssueSeverity(issue: {
  issueType: string;
  severity: string;
}): IssueSeverity {
  const descriptor = getIssueDescriptor(issue.issueType);
  if (descriptor) return descriptor.severity;
  return issue.severity === "critical" || issue.severity === "warning"
    ? issue.severity
    : "info";
}

function groupIssues(issues: AuditIssueRow[]): IssueGroup[] {
  const groups = new Map<string, IssueGroup>();
  for (const issue of issues) {
    let group = groups.get(issue.issueType);
    if (!group) {
      const descriptor = getIssueDescriptor(issue.issueType);
      group = {
        issueType: issue.issueType,
        severity: resolveIssueSeverity(issue),
        title: descriptor?.title ?? issue.issueType,
        explanation: descriptor?.explanation ?? "",
        howToFix: descriptor?.howToFix ?? "",
        issues: [],
      };
      groups.set(issue.issueType, group);
    }
    group.issues.push(issue);
  }

  return sort(
    Array.from(groups.values()),
    (a, b) =>
      ISSUE_SEVERITY_ORDER[a.severity] - ISSUE_SEVERITY_ORDER[b.severity] ||
      b.issues.length - a.issues.length,
  );
}

export function IssuesView({
  issues,
  pages,
  tabs,
}: {
  issues: AuditIssueRow[];
  pages: AuditResultsData["pages"];
  tabs: ReactNode;
}) {
  const groups = useMemo(() => groupIssues(issues), [issues]);

  const sections = useMemo(
    () =>
      (["critical", "warning", "info"] as const)
        .map((severity) => ({
          severity,
          groups: groups.filter((group) => group.severity === severity),
        }))
        .filter((section) => section.groups.length > 0),
    [groups],
  );

  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      {tabs}
      {issues.length === 0 ? (
        <EmptyState
          variant="plain"
          title="No issues recorded for this audit."
          description="Either the site is in great shape, or this audit ran before issue checks existed — run a new audit to get the full report."
        />
      ) : (
        <div>
          {sections.map((section) => (
            <IssueSection
              key={section.severity}
              section={section}
              pages={pages}
              allIssues={issues}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function IssueSection({
  section,
  pages,
  allIssues,
}: {
  section: { severity: IssueSeverity; groups: IssueGroup[] };
  pages: AuditResultsData["pages"];
  allIssues: AuditIssueRow[];
}) {
  const issueCount = section.groups.reduce(
    (sum, group) => sum + group.issues.length,
    0,
  );

  return (
    <div className="border-t border-border first:border-t-0">
      <div className="flex items-center gap-2 bg-muted/50 px-4 py-1.5 border-b border-border">
        <SeverityBadge severity={section.severity}>
          {SEVERITY_LABEL[section.severity]}
          <span className="tabular-nums">{issueCount}</span>
        </SeverityBadge>
      </div>
      <div className="divide-y divide-border">
        {section.groups.map((group) => (
          <IssueRow
            key={group.issueType}
            group={group}
            pages={pages}
            allIssues={allIssues}
          />
        ))}
      </div>
    </div>
  );
}

function IssueRow({
  group,
  pages,
  allIssues,
}: {
  group: IssueGroup;
  pages: AuditResultsData["pages"];
  allIssues: AuditIssueRow[];
}) {
  const [open, setOpen] = useState(false);

  return (
    <div
      className={
        open
          ? `border-l-2 ${SEVERITY_RULE[group.severity]} bg-muted/30`
          : "border-l-2 border-l-transparent"
      }
    >
      <button
        type="button"
        className="w-full flex items-center gap-3 px-4 py-2.5 text-left hover:bg-muted/50 transition-colors"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
      >
        <span className="text-sm font-medium flex-1 min-w-0 truncate">
          {group.title}
        </span>
        <span className="text-xs tabular-nums text-muted-foreground shrink-0">
          {group.issues.length} {group.issues.length === 1 ? "page" : "pages"}
        </span>
        <ChevronRight
          className={`size-4 shrink-0 text-muted-foreground transition-transform ${
            open ? "rotate-90" : ""
          }`}
        />
      </button>

      {open && (
        <div className="pl-4 pr-4 pb-4 pt-0.5 space-y-3">
          {group.explanation && (
            <p className="text-sm text-muted-foreground max-w-prose">
              {group.explanation}
            </p>
          )}
          {group.howToFix && (
            <p className="text-sm max-w-prose">
              <span className="font-medium">How to fix: </span>
              <span className="text-foreground/80">{group.howToFix}</span>
            </p>
          )}
          {group.issueType === "orphan-page" ? (
            <OrphanPageRescue
              issues={group.issues}
              pages={pages}
              allIssues={allIssues}
            />
          ) : null}
          <AffectedUrlList issues={group.issues} />
        </div>
      )}
    </div>
  );
}

function OrphanPageRescue({
  issues,
  pages,
  allIssues,
}: {
  issues: AuditIssueRow[];
  pages: AuditResultsData["pages"];
  allIssues: AuditIssueRow[];
}) {
  const orphanIssues = issues.slice(0, MAX_RENDERED_URLS);
  return (
    <div className="space-y-2 rounded-md border border-border bg-card p-3">
      <div>
        <p className="text-sm font-medium">Suggested pages to link from</p>
        <p className="text-xs text-muted-foreground">
          Matched from titles and URLs in this crawl. Review page context before
          adding a link.
        </p>
      </div>
      {orphanIssues.map((issue) => (
        <OrphanPageSuggestion
          key={issue.id}
          issue={issue}
          pages={pages}
          allIssues={allIssues}
        />
      ))}
      {issues.length > orphanIssues.length ? (
        <p className="text-xs text-muted-foreground">
          Suggestions shown for the first {orphanIssues.length} orphan pages.
        </p>
      ) : null}
    </div>
  );
}

function OrphanPageSuggestion({
  issue,
  pages,
  allIssues,
}: {
  issue: AuditIssueRow;
  pages: AuditResultsData["pages"];
  allIssues: AuditIssueRow[];
}) {
  const [open, setOpen] = useState(false);
  const candidates = open
    ? getOrphanLinkCandidates(issue.pageUrl, pages, allIssues)
    : [];

  return (
    <div className="space-y-1.5 border-t border-border pt-2 first:border-t-0 first:pt-0">
      <p className="break-all text-xs font-medium">Orphan: {issue.pageUrl}</p>
      <Button
        type="button"
        size="xs"
        variant="ghost"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
      >
        {open ? "Hide suggestions" : "Show suggested pages"}
      </Button>
      {open ? (
        candidates.length ? (
          <ul className="space-y-1.5">
            {candidates.map((candidate) => (
              <li
                key={candidate.sourceUrl}
                className="flex flex-wrap items-center gap-2 text-xs"
              >
                <a
                  href={candidate.sourceUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="min-w-0 flex-1 break-all underline underline-offset-4"
                >
                  {candidate.sourceTitle}
                </a>
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  onClick={() =>
                    void copyLinkSnippet(
                      candidate.targetUrl,
                      candidate.targetTitle,
                    )
                  }
                >
                  Copy link snippet
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-xs text-muted-foreground">
            No closely related source page was found in this crawl.
          </p>
        )
      ) : null}
    </div>
  );
}

async function copyLinkSnippet(targetUrl: string, targetTitle: string) {
  if (!navigator.clipboard?.writeText) {
    toast.error("Clipboard access is not available in this browser");
    return;
  }
  const snippet = `<a href="${escapeHtml(targetUrl)}">${escapeHtml(targetTitle)}</a>`;
  try {
    await navigator.clipboard.writeText(snippet);
    toast.success("Link snippet copied");
  } catch {
    toast.error("Could not copy link snippet");
  }
}

function escapeHtml(value: string) {
  return value.replace(/[&<>"']/g, (character) => {
    switch (character) {
      case "&":
        return "&amp;";
      case "<":
        return "&lt;";
      case ">":
        return "&gt;";
      case '"':
        return "&quot;";
      default:
        return "&#39;";
    }
  });
}

function AffectedUrlList({ issues }: { issues: AuditIssueRow[] }) {
  const rendered = issues.slice(0, MAX_RENDERED_URLS);
  const remaining = issues.length - rendered.length;

  return (
    <div className="max-h-[320px] overflow-y-auto rounded border border-border bg-card">
      {rendered.map((issue) => (
        <div
          key={issue.id}
          className="px-3 py-1.5 text-sm flex flex-col gap-0.5 border-b border-border last:border-b-0"
        >
          <a
            className="text-foreground/80 truncate hover:underline"
            href={issue.pageUrl}
            target="_blank"
            rel="noreferrer"
            title={issue.pageUrl}
          >
            {issue.pageUrl}
          </a>
          <IssueDetails detailsJson={issue.detailsJson} />
        </div>
      ))}
      {remaining > 0 && (
        <div className="px-3 py-2 text-xs text-muted-foreground">
          …and {remaining} more — export the issues CSV for the full list.
        </div>
      )}
    </div>
  );
}

// AuditRepository is the only writer and always stores JSON.stringify of a
// details object, so the parse cannot throw.
function parseDetails(detailsJson: string): Array<[string, unknown]> | null {
  const parsed: unknown = JSON.parse(detailsJson);
  if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) {
    return Object.entries(parsed);
  }
  return null;
}

function IssueDetails({ detailsJson }: { detailsJson: string | null }) {
  const details = useMemo(
    () => (detailsJson ? parseDetails(detailsJson) : null),
    [detailsJson],
  );

  if (!details) return null;

  const entries = details.filter(
    ([, value]) => value !== null && value !== undefined,
  );
  if (entries.length === 0) return null;

  return (
    <span className="text-xs text-muted-foreground truncate">
      {entries
        .map(([key, value]) => {
          const rendered = Array.isArray(value)
            ? value.join(" → ")
            : String(value);
          return `${key}: ${rendered}`;
        })
        .join(" · ")}
    </span>
  );
}
