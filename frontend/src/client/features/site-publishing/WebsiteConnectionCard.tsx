import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, ExternalLink } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { ErrorState } from "@/client/components/ErrorState";
import { QueryError } from "@/client/components/QueryState";
import { Spinner } from "@/client/components/Spinner";
import { Button } from "@/client/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/client/components/ui/field";
import { Input } from "@/client/components/ui/input";
import { billingStatusQueryOptions } from "@/client/features/billing/plan/planApi";
import { useCanManageBilling } from "@/client/features/team/organizationQueries";
import { getStandardErrorMessage } from "@/client/lib/error-messages";
import {
  connectWordPress,
  disconnectSite,
  siteConnectionQueryOptions,
  type SiteConnection,
} from "./sitePublishingApi";

/**
 * Connects the project's WordPress site so content-editor articles can be
 * published to it. Used on the project's Integrations settings and in
 * onboarding.
 */
export function WebsiteConnectionCard({ projectId }: { projectId: string }) {
  const connectionQuery = useQuery(siteConnectionQueryOptions(projectId));
  const statusQuery = useQuery(billingStatusQueryOptions);
  const canManage = useCanManageBilling();

  if (connectionQuery.isPending) {
    return <Spinner size="sm" label="Checking your website…" />;
  }
  if (connectionQuery.isError) {
    return (
      <QueryError
        error={connectionQuery.error}
        fallback="Couldn't load your website connection."
        onRetry={() => void connectionQuery.refetch()}
        isRetrying={connectionQuery.isFetching}
      />
    );
  }
  if (connectionQuery.data.connected) {
    return (
      <ConnectedSite
        projectId={projectId}
        connection={connectionQuery.data}
        canManage={canManage}
      />
    );
  }
  // The server enforces the plan; this only saves a Free workspace from
  // filling in a form that would be refused.
  const status = statusQuery.data;
  if (status && status.plan === "free" && !status.unlimited) {
    return (
      <Notice>
        Publishing to your website is part of Basic, Pro and Agency.{" "}
        <Link to="/billing/plan" className="underline underline-offset-4">
          See plans
        </Link>
      </Notice>
    );
  }
  if (!canManage) {
    return (
      <Notice>
        Ask a workspace owner or admin to connect your WordPress site.
      </Notice>
    );
  }
  return <ConnectForm projectId={projectId} />;
}

function ConnectedSite({
  projectId,
  connection,
  canManage,
}: {
  projectId: string;
  connection: SiteConnection;
  canManage: boolean;
}) {
  const queryClient = useQueryClient();
  const disconnect = useMutation({
    mutationFn: () => disconnectSite(projectId),
    meta: { errorToast: false },
    onSuccess: (next) => {
      queryClient.setQueryData(
        siteConnectionQueryOptions(projectId).queryKey,
        next,
      );
      toast.success("Website disconnected. Posts already published stay.");
    },
  });

  return (
    <div className="space-y-3 rounded-lg border border-border bg-card p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="flex items-center gap-2 text-sm font-medium">
            <Check className="size-4 text-primary" />
            {connection.siteName || connection.siteUrl}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            WordPress ·{" "}
            <a
              href={connection.siteUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 underline underline-offset-4"
            >
              {connection.siteUrl}
              <ExternalLink className="size-3" />
            </a>{" "}
            · signed in as {connection.username}
          </p>
        </div>
        {canManage ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => disconnect.mutate()}
            disabled={disconnect.isPending}
          >
            {disconnect.isPending ? "Disconnecting…" : "Disconnect"}
          </Button>
        ) : null}
      </div>
      {disconnect.isError ? (
        <ErrorState
          variant="inline"
          message={getStandardErrorMessage(disconnect.error)}
        />
      ) : null}
    </div>
  );
}

function ConnectForm({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [siteUrl, setSiteUrl] = useState("");
  const [username, setUsername] = useState("");
  const [applicationPassword, setApplicationPassword] = useState("");
  const connect = useMutation({
    mutationFn: () =>
      connectWordPress(projectId, { siteUrl, username, applicationPassword }),
    meta: { errorToast: false },
    onSuccess: (connection) => {
      setApplicationPassword("");
      queryClient.setQueryData(
        siteConnectionQueryOptions(projectId).queryKey,
        connection,
      );
      toast.success("Website connected.");
    },
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    connect.mutate();
  };

  return (
    <form
      onSubmit={submit}
      className="space-y-4 rounded-lg border border-border bg-card p-4"
    >
      <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
        <li>Open your WordPress admin and go to Users → Profile.</li>
        <li>
          Under <span className="font-medium">Application Passwords</span>, type
          “Uplumo” and click Add New Application Password.
        </li>
        <li>Copy the password WordPress shows and paste it below.</li>
      </ol>
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="site-url">Website address</FieldLabel>
          <Input
            id="site-url"
            placeholder="example.com"
            autoComplete="url"
            value={siteUrl}
            onChange={(event) => setSiteUrl(event.target.value)}
            required
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="site-username">WordPress username</FieldLabel>
          <Input
            id="site-username"
            autoComplete="off"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            required
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="site-app-password">
            Application password
          </FieldLabel>
          <Input
            id="site-app-password"
            type="password"
            autoComplete="off"
            placeholder="abcd efgh ijkl mnop qrst uvwx"
            value={applicationPassword}
            onChange={(event) => setApplicationPassword(event.target.value)}
            required
          />
          <FieldDescription>
            Not your login password. Uplumo can only write posts and upload
            images, and you can revoke it in WordPress at any time.
          </FieldDescription>
        </Field>
      </FieldGroup>
      {connect.isError ? (
        <ErrorState
          variant="inline"
          message={getStandardErrorMessage(connect.error)}
        />
      ) : null}
      <Button type="submit" disabled={connect.isPending}>
        {connect.isPending ? "Checking your site…" : "Connect WordPress"}
      </Button>
    </form>
  );
}

function Notice({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
      {children}
    </p>
  );
}
