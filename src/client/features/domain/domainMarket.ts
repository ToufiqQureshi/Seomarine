import type { ProjectMarket } from "@/client/features/projects/types";
import { useProjectMarket } from "@/client/features/projects/useProjectMarket";
import { resolveLabsMarket } from "@/shared/keyword-locations";

/**
 * The market Labs can serve for this project: the caller's location, or the
 * project default when Labs serves it, otherwise the United States. The Go API
 * sends exactly what it receives, so the client resolves it first.
 */
export function resolveDomainMarket(
  project: ProjectMarket,
  locationCode: number | undefined,
) {
  return resolveLabsMarket({ locationCode }, project);
}

/** The resolved market, or undefined until the projects list has loaded. */
export function useDomainMarket(
  projectId: string,
  locationCode: number | undefined,
) {
  const project = useProjectMarket(projectId);
  return project ? resolveDomainMarket(project, locationCode) : undefined;
}
