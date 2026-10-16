import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@/client/components/ui/button";
import { Input } from "@/client/components/ui/input";
import { Textarea } from "@/client/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/client/components/ui/dialog";
import { captureClientEvent } from "@/client/lib/posthog";
import { saveReport } from "@/client/features/reports/reportApi";
import { reportsQueryKey } from "@/client/features/reports/shared";
import { buildManualReportHtml } from "@/client/features/reports/manualReportHtml";

const MAX_BODY_CHARACTERS = 60_000;

export function ManualReportForm({
  projectId,
  onClose,
}: {
  projectId: string;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const [body, setBody] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const saveMutation = useMutation({
    mutationFn: () =>
      saveReport({
        data: {
          projectId,
          title: title.trim(),
          summary: summary.trim(),
          html: buildManualReportHtml({ title, summary, body }),
          skill: "Manual",
        },
      }),
    onSuccess: (result) => {
      captureClientEvent("report:created", {
        project_id: projectId,
        report_id: result.reportId,
        source: "manual",
      });
      toast.success("Report saved");
      void queryClient.invalidateQueries({
        queryKey: reportsQueryKey(projectId),
      });
      void navigate({
        to: "/p/$projectId/reports/$reportId",
        params: { projectId, reportId: result.reportId },
      });
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : "Could not save the report",
      );
    },
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const cleanTitle = title.trim();
    const cleanSummary = summary.trim();
    const cleanBody = body.trim();
    if (!cleanTitle || !cleanSummary || !cleanBody) {
      setFormError("Add a title, a short summary, and the report details.");
      return;
    }
    if (cleanTitle.length > 120) {
      setFormError("Keep the title to 120 characters or fewer.");
      return;
    }
    if (cleanSummary.length > 2_500) {
      setFormError("Keep the summary to 2,500 characters or fewer.");
      return;
    }
    if (cleanBody.length > MAX_BODY_CHARACTERS) {
      setFormError("Keep report details to 60,000 characters or fewer.");
      return;
    }
    setFormError(null);
    saveMutation.mutate();
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !saveMutation.isPending) onClose();
      }}
    >
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Write a report</DialogTitle>
          <DialogDescription>
            Create a client-ready report yourself. AI-generated reports remain
            available through your connected assistant.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-5">
          <div className="space-y-2">
            <label htmlFor="report-title" className="text-sm font-medium">
              Report title
            </label>
            <Input
              id="report-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              maxLength={120}
              placeholder="Monthly SEO update — September"
              required
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="report-summary" className="text-sm font-medium">
              Short summary
            </label>
            <Textarea
              id="report-summary"
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              maxLength={2_500}
              rows={3}
              placeholder="The main result and what the client should know."
              required
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="report-body" className="text-sm font-medium">
              Report details
            </label>
            <Textarea
              id="report-body"
              value={body}
              onChange={(event) => setBody(event.target.value)}
              maxLength={MAX_BODY_CHARACTERS}
              rows={18}
              placeholder={`What we worked on\n\nWhat changed and what it means\n\nNext steps`}
              required
              className="min-h-80 leading-relaxed"
            />
            <p className="text-xs text-muted-foreground">
              Plain text is safe to paste from your notes. Separate paragraphs
              with a blank line.
            </p>
          </div>

          {formError ? (
            <p role="alert" className="text-sm text-destructive">
              {formError}
            </p>
          ) : null}

          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={onClose}
              disabled={saveMutation.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={saveMutation.isPending}>
              {saveMutation.isPending ? "Saving…" : "Save report"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
