import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ExternalLink, Send } from "lucide-react";
import { toast } from "sonner";
import { ErrorState } from "@/client/components/ErrorState";
import { Button } from "@/client/components/ui/button";
import { getStandardErrorMessage } from "@/client/lib/error-messages";
import {
  publishDraft,
  siteConnectionQueryOptions,
  sitePublicationQueryOptions,
  type SitePublication,
} from "./sitePublishingApi";

/**
 * Sends the saved draft to the project's WordPress site, as a WordPress
 * draft or published. Publishing again updates the same post.
 */
export function PublishToSite({
  projectId,
  draftId,
  hasUnsavedChanges,
  blocksPublish,
  targetKeyword,
  schema,
}: {
  projectId: string;
  draftId: string | undefined;
  hasUnsavedChanges: boolean;
  blocksPublish: boolean;
  targetKeyword: string;
  schema?: { article: boolean; faq: boolean };
}) {
  const connectionQuery = useQuery(siteConnectionQueryOptions(projectId));
  const connection = connectionQuery.data;

  return (
    <section className="space-y-3 rounded-lg border border-border bg-card p-4">
      <h2 className="text-sm font-medium">Publish to your website</h2>
      {connectionQuery.isPending ? null : !connection?.connected ? (
        <p className="text-sm text-muted-foreground">
          Connect your WordPress site to publish this article in one click.{" "}
          <Link
            to="/p/$projectId/settings/integrations"
            params={{ projectId }}
            hash="website"
            className="underline underline-offset-4"
          >
            Connect website
          </Link>
        </p>
      ) : !draftId ? (
        <p className="text-sm text-muted-foreground">
          Save the draft first, then publish it to{" "}
          {connection.siteName || connection.siteUrl}.
        </p>
      ) : (
        <PublishActions
          key={draftId}
          projectId={projectId}
          draftId={draftId}
          siteName={connection.siteName || connection.siteUrl || "your site"}
          hasUnsavedChanges={hasUnsavedChanges}
          blocksPublish={blocksPublish}
          targetKeyword={targetKeyword}
          schema={schema}
        />
      )}
    </section>
  );
}

function PublishActions({
  projectId,
  draftId,
  siteName,
  hasUnsavedChanges,
  blocksPublish,
  targetKeyword,
  schema,
}: {
  projectId: string;
  draftId: string;
  siteName: string;
  hasUnsavedChanges: boolean;
  blocksPublish: boolean;
  targetKeyword: string;
  schema?: { article: boolean; faq: boolean };
}) {
  const queryClient = useQueryClient();
  const publicationOptions = sitePublicationQueryOptions(projectId, draftId);
  const publication = useQuery(publicationOptions).data;
  const publish = useMutation({
    mutationFn: (status: "draft" | "publish") =>
      publishDraft(projectId, draftId, status, schema),
    meta: { errorToast: false },
    onSuccess: (next) => {
      queryClient.setQueryData(publicationOptions.queryKey, next);
      toast.success(
        next.status === "publish"
          ? `Live on ${siteName}.`
          : `Saved as a draft on ${siteName}.`,
      );
    },
  });
  const published = publication?.published === true;

  return (
    <div className="space-y-3">
      {published ? <PublishedLine publication={publication} /> : null}
      {published && publication.status === "publish" ? (
        <TrackArticle projectId={projectId} targetKeyword={targetKeyword} />
      ) : null}
      {hasUnsavedChanges ? (
        <p className="text-sm text-muted-foreground">
          Save your changes first: the saved version is what gets published.
        </p>
      ) : null}
      {blocksPublish ? (
        <p role="alert" className="text-sm text-destructive">
          Fix keyword stuffing before publishing this article.
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={publish.isPending || hasUnsavedChanges}
          onClick={() => publish.mutate("draft")}
        >
          {published ? "Update as draft" : "Send as draft"}
        </Button>
        <Button
          size="sm"
          disabled={publish.isPending || hasUnsavedChanges || blocksPublish}
          onClick={() => publish.mutate("publish")}
        >
          <Send aria-hidden />
          {publish.isPending
            ? "Publishing…"
            : published
              ? "Update and publish"
              : "Publish now"}
        </Button>
      </div>
      {publication?.schema?.added === false && publication.schema.note ? (
        <p className="text-xs text-muted-foreground">
          {publication.schema.note}
        </p>
      ) : null}
      {publish.isError ? (
        <ErrorState
          variant="inline"
          message={getStandardErrorMessage(publish.error)}
        />
      ) : null}
    </div>
  );
}

function PublishedLine({ publication }: { publication: SitePublication }) {
  const when = publication.updatedAt
    ? new Date(publication.updatedAt).toLocaleString("en-IN", {
        dateStyle: "medium",
        timeStyle: "short",
      })
    : "";
  return (
    <p className="text-sm text-muted-foreground">
      {publication.status === "publish" ? "Published" : "WordPress draft"}
      {when ? `, last sent ${when}` : ""}.{" "}
      {publication.url ? (
        <a
          href={publication.url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 underline underline-offset-4"
        >
          View on site <ExternalLink className="size-3" />
        </a>
      ) : null}
    </p>
  );
}

/**
 * After an article goes live, the two follow-ups that tell the writer if it
 * worked. Both open the existing tools, which show their own credit cost
 * before anything is checked.
 */
function TrackArticle({
  projectId,
  targetKeyword,
}: {
  projectId: string;
  targetKeyword: string;
}) {
  const topic = targetKeyword.trim();
  return (
    <div className="space-y-1.5 rounded-md bg-muted px-3 py-2.5 text-sm">
      <p className="font-medium">Track this article</p>
      <Link
        to="/p/$projectId/rank-tracking"
        params={{ projectId }}
        className="block text-primary underline-offset-4 hover:underline"
      >
        Track its Google position →
      </Link>
      {topic ? (
        <Link
          to="/p/$projectId/prompt-explorer"
          params={{ projectId }}
          search={{ q: `What is ${topic}?` }}
          className="block text-primary underline-offset-4 hover:underline"
        >
          Check if AI answers mention you →
        </Link>
      ) : null}
    </div>
  );
}
