import { Link } from "@tanstack/react-router";
import { EmptyState } from "@/client/components/EmptyState";
import { Button } from "@/client/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/client/components/ui/table";
import { formatCount } from "@/client/features/search-performance/SearchPerformanceColumns";
import {
  buildContentRefreshQueue,
  type RefreshQuery,
} from "@/client/features/search-performance/contentRefreshQueue";

function safePageUrl(value: string) {
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:"
      ? url.toString()
      : undefined;
  } catch {
    return undefined;
  }
}

export function ContentRefreshQueue({
  projectId,
  rows,
}: {
  projectId: string;
  rows: RefreshQuery[];
}) {
  const pages = buildContentRefreshQueue(rows);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Content refresh queue</CardTitle>
        <CardDescription>
          Pages found in the report&apos;s 100 highest-impression
          striking-distance queries, with at least 50 impressions and average
          positions 5–20. These are review candidates, not proof that a page is
          outdated.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {pages.length === 0 ? (
          <EmptyState
            title="No refresh candidates in this range"
            description="Try a wider date range or check again after Search Console collects more data."
          />
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Page</TableHead>
                  <TableHead>Impressions</TableHead>
                  <TableHead>Clicks</TableHead>
                  <TableHead>Avg. position</TableHead>
                  <TableHead>Query themes</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {pages.slice(0, 25).map((item) => (
                  <TableRow key={item.page}>
                    <TableCell className="max-w-72 break-all font-medium">
                      {item.page}
                    </TableCell>
                    <TableCell>{formatCount(item.impressions)}</TableCell>
                    <TableCell>{formatCount(item.clicks)}</TableCell>
                    <TableCell>{item.averagePosition.toFixed(1)}</TableCell>
                    <TableCell className="max-w-72">
                      {item.queries.map((query) => query.query).join(" · ")}
                    </TableCell>
                    <TableCell>
                      {safePageUrl(item.page) ? (
                        <Button
                          size="xs"
                          variant="outline"
                          nativeButton={false}
                          render={
                            <a
                              href={safePageUrl(item.page)}
                              target="_blank"
                              rel="noreferrer"
                            />
                          }
                        >
                          Review page
                        </Button>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
        {pages.length > 25 ? (
          <p className="mt-3 text-xs text-muted-foreground">
            Showing 25 of {pages.length} candidate pages from the current Search
            Console report.
          </p>
        ) : null}
        <p className="mt-3 text-xs text-muted-foreground">
          Use the query themes to decide whether the page needs a factual
          update, clearer coverage, or a stronger search snippet. Keep useful
          content intact.
          <Link
            to="/p/$projectId/content-editor"
            params={{ projectId }}
            className="ml-1 underline underline-offset-4"
          >
            Open Content Editor
          </Link>
        </p>
      </CardContent>
    </Card>
  );
}
