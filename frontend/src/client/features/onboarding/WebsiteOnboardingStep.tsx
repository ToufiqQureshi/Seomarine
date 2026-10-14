import { useQuery } from "@tanstack/react-query";
import { QueryError } from "@/client/components/QueryState";
import { Spinner } from "@/client/components/Spinner";
import { WizardFooter } from "@/client/features/onboarding/WizardFooter";
import { projectsQueryOptions } from "@/client/features/projects/projectQueries";
import { WebsiteConnectionCard } from "@/client/features/site-publishing/WebsiteConnectionCard";
import { siteConnectionQueryOptions } from "@/client/features/site-publishing/sitePublishingApi";

/**
 * Optional onboarding step: connect the first project's WordPress site so
 * articles written in Uplumo can be published to it. The same card lives on
 * the project's Integrations settings.
 */
export function WebsiteOnboardingStep({
  onNext,
  onBack,
  onSkip,
}: {
  onNext: () => void;
  onBack: () => void;
  onSkip: () => void;
}) {
  const projectsQuery = useQuery(projectsQueryOptions());
  const project = projectsQuery.data?.[0];
  const connectionQuery = useQuery({
    ...siteConnectionQueryOptions(project?.id ?? ""),
    enabled: Boolean(project),
  });
  const connected = connectionQuery.data?.connected === true;

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Publish straight to your website?
        </h1>
        <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
          Connect your WordPress site and every article you write in Uplumo can
          go live in one click, images included. You can also do this later in
          Settings → Integrations.
        </p>
      </div>

      {project ? (
        <WebsiteConnectionCard projectId={project.id} />
      ) : projectsQuery.isError ? (
        <QueryError
          error={projectsQuery.error}
          fallback="Couldn't load your project."
          onRetry={() => void projectsQuery.refetch()}
          isRetrying={projectsQuery.isFetching}
        />
      ) : (
        <Spinner size="sm" label="Checking…" />
      )}

      <WizardFooter
        onBack={onBack}
        onSkip={connected ? undefined : onSkip}
        skipLabel="Skip for now"
        onContinue={connected ? onNext : undefined}
      />
    </div>
  );
}
