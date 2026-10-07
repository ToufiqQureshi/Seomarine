import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { BackLink, PageHeader } from "@/client/components/PageHeader";
import { projectsQueryOptions } from "./projectQueries";

export function ProjectPageHeader({
  projectId,
  title,
  showBackLink = false,
  actions,
}: {
  projectId: string;
  title: string;
  showBackLink?: boolean;
  actions?: ReactNode;
}) {
  const projectsQuery = useQuery(projectsQueryOptions());
  const project = projectsQuery.data?.find((entry) => entry.id === projectId);

  return (
    <PageHeader
      title={title}
      description={project?.name ?? " "}
      actions={actions}
      backLink={
        showBackLink ? <BackLink to="/projects">Projects</BackLink> : undefined
      }
    />
  );
}
