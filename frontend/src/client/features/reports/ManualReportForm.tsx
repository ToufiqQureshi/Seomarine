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
import { saveReport, shareReport } from "@/client/features/reports/reportApi";
import { reportsQueryKey } from "@/client/features/reports/shared";
import {
  isSafeEvidenceUrl,
  WorkLogFields,
} from "@/client/features/reports/ManualReportWorkLogFields";
import {
  buildManualReportHtml,
  buildManualWorkLogHtml,
} from "@/client/features/reports/manualReportHtml";

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
  const [mode, setMode] = useState<"report" | "work-log">("report");
  const [period, setPeriod] = useState("");
  const [workCompleted, setWorkCompleted] = useState("");
  const [outcomes, setOutcomes] = useState("");
  const [evidence, setEvidence] = useState("");
  const [nextSteps, setNextSteps] = useState("");
  const [shareWithClient, setShareWithClient] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const saveMutation = useMutation({
    mutationFn: () =>
      saveReport({
        data: {
          projectId,
          title: title.trim(),
          summary: summary.trim(),
          html:
            mode === "work-log"
              ? buildManualWorkLogHtml({
                  title,
                  summary,
                  period,
                  workCompleted,
                  outcomes,
                  evidenceUrls: evidence
                    .split("\n")
                    .map((line) => line.trim())
                    .filter(Boolean),
                  nextSteps,
                })
              : buildManualReportHtml({ title, summary, body }),
          skill: mode === "work-log" ? "Manual work log" : "Manual",
        },
      }),
    onSuccess: async (result) => {
      let shared = false;
      if (mode === "work-log" && shareWithClient) {
        try {
          await shareReport({ data: { projectId, reportId: result.reportId } });
          shared = true;
        } catch {
          toast.error(
            "Work log saved, but the client review link could not be created.",
          );
        }
      }
      captureClientEvent("report:created", {
        project_id: projectId,
        report_id: result.reportId,
        source: "manual",
      });
      if (shared) {
        toast.success("Work log saved and shared for client review");
      } else if (!shareWithClient || mode !== "work-log") {
        toast.success(mode === "work-log" ? "Work log saved" : "Report saved");
      }
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
    const cleanWork = workCompleted.trim();
    if (
      !cleanTitle ||
      !cleanSummary ||
      (mode === "report" && !cleanBody) ||
      (mode === "work-log" && (!cleanWork || !period))
    ) {
      setFormError(
        mode === "work-log"
          ? "Add a title, summary, reporting month, and completed work."
          : "Add a title, a short summary, and the report details.",
      );
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
    if (mode === "work-log") {
      const evidenceUrls = evidence
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean);
      if (
        evidenceUrls.length > 10 ||
        evidenceUrls.some((value) => !isSafeEvidenceUrl(value))
      ) {
        setFormError(
          "Use up to 10 evidence links. Each link must start with http:// or https:// and contain no login details.",
        );
        return;
      }
      if (
        workCompleted.length +
          outcomes.length +
          nextSteps.length +
          evidence.length >
        20_000
      ) {
        setFormError("Keep work log details under 20,000 characters in total.");
        return;
      }
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
        <form onSubmit={submit}>
          <fieldset
            disabled={saveMutation.isPending}
            className="m-0 min-w-0 space-y-5 border-0 p-0"
          >
            <legend className="sr-only">Report fields</legend>
            <div
              className="flex flex-wrap gap-2"
              role="group"
              aria-label="Report type"
            >
              <Button
                type="button"
                size="sm"
                variant={mode === "report" ? "secondary" : "outline"}
                aria-pressed={mode === "report"}
                onClick={() => setMode("report")}
              >
                Standard report
              </Button>
              <Button
                type="button"
                size="sm"
                variant={mode === "work-log" ? "secondary" : "outline"}
                aria-pressed={mode === "work-log"}
                onClick={() => setMode("work-log")}
              >
                Client work log
              </Button>
            </div>
            <div className="space-y-2">
              <label htmlFor="report-title" className="text-sm font-medium">
                Report title
              </label>
              <Input
                id="report-title"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                maxLength={120}
                placeholder={
                  mode === "work-log"
                    ? "SEO work log — September"
                    : "Monthly SEO update — September"
                }
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
            {mode === "work-log" ? (
              <WorkLogFields
                period={period}
                setPeriod={setPeriod}
                workCompleted={workCompleted}
                setWorkCompleted={setWorkCompleted}
                outcomes={outcomes}
                setOutcomes={setOutcomes}
                evidence={evidence}
                setEvidence={setEvidence}
                nextSteps={nextSteps}
                setNextSteps={setNextSteps}
                shareWithClient={shareWithClient}
                setShareWithClient={setShareWithClient}
              />
            ) : null}
            {mode === "report" ? (
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
                  Plain text is safe to paste from your notes. Separate
                  paragraphs with a blank line.
                </p>
              </div>
            ) : null}
          </fieldset>

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
              {saveMutation.isPending
                ? "Saving…"
                : mode === "work-log"
                  ? "Save work log"
                  : "Save report"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

