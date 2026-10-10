import { queryOptions } from "@tanstack/react-query";
import { queryClient } from "@/client/tanstack-db";
import { listSamSessions } from "@/client/features/sam/samApi";

export const samSessionsQueryOptions = (projectId: string) =>
  queryOptions({
    queryKey: ["samSessions", projectId],
    queryFn: () => listSamSessions({ data: { projectId } }),
  });

export function invalidateSamSessions(projectId: string) {
  return queryClient.invalidateQueries({
    queryKey: samSessionsQueryOptions(projectId).queryKey,
  });
}
