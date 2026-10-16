import { useQueries } from "@tanstack/react-query";
import { useState } from "react";
import { ArrowDown, ArrowUp, GitCompareArrows } from "lucide-react";
import { Button } from "@/client/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import {
  getAuditResults,
  type AuditHistory,
} from "@/client/features/audit/auditApi";
import { compareAuditResults, latestComparablePair } from "@/client/features/audit/compare";
import { formatDate } from "@/client/features/audit/shared";

export function AuditChangeGuard({
  projectId,
  history,
}: {
  projectId: string;
  history: AuditHistory;
}) {
  const [comparing, setComparing] = useState(false);
  const pair = latestComparablePair(history);
  const results = useQueries({
    queries: pair.map((audit) => ({
      queryKey: ["audit", projectId, audit.id, "results"],
      queryFn: () =>
        getAuditResults({ data: { projectId, auditId: audit.id } }),
      enabled: comparing,
      staleTime: 60_000,
    })),
  });
  const isLoading = comparing && results.some((query) => query.isLoading);
  const hasError = comparing && results.some((query) => query.isError);
  const older = results[1]?.data;
  const newer = results[0]?.data;
  const diff = older && newer ? compareAuditResults(older, newer) : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitCompareArrows className="size-4" /> SEO change guard
        </CardTitle>
        <CardDescription>
          Compare the two latest completed crawls of the same start URL. Review
          crawl coverage before treating missing URLs as removed.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {pair.length < 2 ? (
          <p className="text-sm text-muted-foreground">
            Run two completed audits with the same start URL to see SEO changes
            here.
          </p>
        ) : (
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm text-muted-foreground">
              {formatDate(pair[1].startedAt)} → {formatDate(pair[0].startedAt)}
            </p>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setComparing(true)}
              disabled={isLoading}
            >
              {isLoading
                ? "Comparing…"
                : comparing
                  ? "Refresh comparison"
                  : "Compare audits"}
            </Button>
          </div>
        )}
        {hasError ? (
          <p role="alert" className="text-sm text-destructive">
            Could not load both audits. Try again; your saved audit data is
            unchanged.
          </p>
        ) : null}
        {diff ? (
          <div className="space-y-3" aria-live="polite">
            <div className="flex flex-wrap gap-x-5 gap-y-1 text-sm">
              <span>{diff.changes.length} SEO field changes</span>
              <span>{diff.addedPages} newly seen URLs</span>
              <span>{diff.removedPages} URLs missing from latest crawl</span>
              <span className="inline-flex items-center gap-1">
                {diff.issueDelta > 0 ? (
                  <ArrowUp className="size-3.5 text-destructive" />
                ) : null}
                {diff.issueDelta < 0 ? (
                  <ArrowDown className="size-3.5 text-success" />
                ) : null}
                {diff.issueDelta === 0
                  ? "No change in issue records"
                  : `${Math.abs(diff.issueDelta)} issue records ${diff.issueDelta > 0 ? "added" : "removed"}`}
              </span>
            </div>
            {diff.changes.length ? (
              <ul className="max-h-72 space-y-2 overflow-auto rounded-md border p-3 text-sm">
                {diff.changes.slice(0, 30).map((change, index) => (
                  <li
                    key={`${change.url}-${change.field}-${index}`}
                    className="space-y-0.5"
                  >
                    <p className="font-medium">
                      {change.field} · {change.url}
                    </p>
                    <p className="break-words text-muted-foreground">
                      <span className="text-foreground">Before:</span>{" "}
                      {change.before}
                      <span className="mx-2">→</span>
                      <span className="text-foreground">After:</span>{" "}
                      {change.after}
                    </p>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground">
                No title, meta, canonical, indexability, or status changes were
                found.
              </p>
            )}
            {diff.changes.length > 30 ? (
              <p className="text-xs text-muted-foreground">
                Showing 30 of {diff.changes.length} changed fields.
              </p>
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

