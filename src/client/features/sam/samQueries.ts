import { queryOptions } from "@tanstack/react-query";
import { queryClient } from "@/client/tanstack-db";
import { listSamMessages, listSamSessions } from "@/client/features/sam/samApi";

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

// The transcript a chat opens with. Never cached across mounts: useChat owns
// the live messages after that, and a stale list would resurrect a reply that
// a rewind deleted.
export const samMessagesQueryOptions = (projectId: string, sessionId: string) =>
  queryOptions({
    queryKey: ["samMessages", projectId, sessionId],
    queryFn: () => listSamMessages({ data: { projectId, sessionId } }),
    gcTime: 0,
    staleTime: 0,
  });
