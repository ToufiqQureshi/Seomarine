import { createFileRoute, Link } from "@tanstack/react-router";
import { PageHeader } from "@/client/components/PageHeader";
import { Button } from "@/client/components/ui/button";
import { Plus } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { ReportsList } from "@/client/features/reports/ReportsList";
import {
  DeleteReportModal,
  reportsQueryKey,
  useDeleteReport,
} from "@/client/features/reports/shared";
import { QueryState } from "@/client/components/QueryState";
import {
  listReports,
  type ReportListItem,
} from "@/client/features/reports/reportApi";
import { REPORT_APP_LIST_LIMIT } from "@/types/schemas/reports";
import { ManualReportForm } from "@/client/features/reports/ManualReportForm";

export const Route = createFileRoute("/_app/p/$projectId/reports/")({
  component: ReportsPage,
});

function ReportsPage() {
  const { projectId } = Route.useParams();
  const [pendingDelete, setPendingDelete] = useState<ReportListItem | null>(
    null,
  );
  const [manualReportOpen, setManualReportOpen] = useState(false);

  const reportsQuery = useQuery({
    queryKey: reportsQueryKey(projectId),
    queryFn: () =>
      listReports({ data: { projectId, limit: REPORT_APP_LIST_LIMIT } }),
    // This page exists to inspect what an agent just wrote; the app-wide
    // 5-minute staleTime would show a pre-save list as current.
    staleTime: 0,
  });

  const deleteMutation = useDeleteReport(projectId, () =>
    setPendingDelete(null),
  );

  return (
    <div className="overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-7xl space-y-4">
        <PageHeader
          title="Reports"
          description="Create reports yourself or review reports saved by your connected AI assistant."
          actions={
            <div className="flex flex-wrap gap-2">
              <Button
                variant="ghost"
                nativeButton={false}
                render={
                  <Link
                    to="/p/$projectId/reports/templates"
                    params={{ projectId }}
                  />
                }
              >
                Templates
              </Button>
              <Button onClick={() => setManualReportOpen(true)}>
                <Plus data-icon="inline-start" />
                Write report
              </Button>
            </div>
          }
        />

        <QueryState query={reportsQuery} errorFallback="Failed to load reports">
          {(data) => (
            <ReportsList
              projectId={projectId}
              reports={data.reports}
              onDelete={setPendingDelete}
            />
          )}
        </QueryState>
      </div>

      {pendingDelete ? (
        <DeleteReportModal
          title={pendingDelete.title}
          isPending={deleteMutation.isPending}
          onClose={() => setPendingDelete(null)}
          onConfirm={() => deleteMutation.mutate(pendingDelete.id)}
        />
      ) : null}
      {manualReportOpen ? (
        <ManualReportForm
          projectId={projectId}
          onClose={() => setManualReportOpen(false)}
        />
      ) : null}
    </div>
  );
}
