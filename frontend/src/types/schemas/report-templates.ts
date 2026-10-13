// Report template limits are enforced by the Go service.

/** A stored template. A plain type, as ReportMetadata: rows come from the repository's own column selection. */
export type ReportTemplate = {
  id: string;
  projectId: string;
  name: string;
  description: string;
  instructions: string;
  createdBy: string;
  createdByUserId: string;
  createdAt: string;
  updatedAt: string;
};
