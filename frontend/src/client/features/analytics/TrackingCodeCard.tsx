import { useMutation } from "@tanstack/react-query";
import { Check, Code, Copy } from "lucide-react";
import { CardShell } from "@/client/components/CardShell";
import { ErrorState } from "@/client/components/ErrorState";
import { Button } from "@/client/components/ui/button";
import { useCopy } from "@/client/hooks/useCopy";
import { getStandardErrorMessage } from "@/client/lib/error-messages";
import { createAnalyticsSite } from "./analyticsApi";

/**
 * Gets the site's tracking snippet (the API returns the existing one on every
 * later call) and shows it with a copy button and install steps.
 */
export function TrackingCodeCard({ projectId }: { projectId: string }) {
  const site = useMutation({
    mutationFn: () => createAnalyticsSite(projectId),
    meta: { errorToast: false },
  });
  const { copied, copy } = useCopy();

  return (
    <CardShell
      title="Install the tracking code"
      icon={<Code className="size-4 text-primary" />}
    >
      <div className="space-y-4 text-sm">
        <ol className="list-decimal space-y-1 pl-5 text-muted-foreground">
          <li>Get your tracking code below.</li>
          <li>
            Paste it inside the <code>&lt;head&gt;</code> of every page (or your
            site builder&rsquo;s &ldquo;custom code&rdquo; box).
          </li>
          <li>Open your site once. Visits show up here within a minute.</li>
        </ol>
        <p className="text-muted-foreground">
          The script is tiny, sets no cookies and stores no personal data, so
          you don&rsquo;t need a cookie banner for it.
        </p>
        {site.data ? (
          <div className="space-y-2">
            <pre className="overflow-x-auto rounded-lg border border-border bg-muted p-3 font-mono text-xs whitespace-pre-wrap break-all">
              {site.data.snippet}
            </pre>
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                void copy(site.data.snippet, "Tracking code copied")
              }
            >
              {copied ? <Check /> : <Copy />}
              {copied ? "Copied" : "Copy code"}
            </Button>
          </div>
        ) : (
          <Button onClick={() => site.mutate()} disabled={site.isPending}>
            {site.isPending ? "Getting your code…" : "Get my tracking code"}
          </Button>
        )}
        {site.isError ? (
          <ErrorState
            variant="inline"
            message={getStandardErrorMessage(site.error)}
            onRetry={() => site.mutate()}
            isRetrying={site.isPending}
          />
        ) : null}
      </div>
    </CardShell>
  );
}
