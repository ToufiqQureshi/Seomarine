import { Input } from "@/client/components/ui/input";
import { Textarea } from "@/client/components/ui/textarea";

export function WorkLogFields({
  period,
  setPeriod,
  workCompleted,
  setWorkCompleted,
  outcomes,
  setOutcomes,
  evidence,
  setEvidence,
  nextSteps,
  setNextSteps,
  shareWithClient,
  setShareWithClient,
}: {
  period: string;
  setPeriod: (value: string) => void;
  workCompleted: string;
  setWorkCompleted: (value: string) => void;
  outcomes: string;
  setOutcomes: (value: string) => void;
  evidence: string;
  setEvidence: (value: string) => void;
  nextSteps: string;
  setNextSteps: (value: string) => void;
  shareWithClient: boolean;
  setShareWithClient: (value: boolean) => void;
}) {
  return (
    <>
      <div className="space-y-2">
        <label htmlFor="worklog-period" className="text-sm font-medium">
          Reporting month
        </label>
        <Input
          id="worklog-period"
          type="month"
          value={period}
          onChange={(event) => setPeriod(event.target.value)}
          required
        />
      </div>
      <WorkLogField
        id="worklog-done"
        label="Work completed"
        value={workCompleted}
        onChange={setWorkCompleted}
        placeholder="List the tasks completed and the problem each one addressed."
        required
      />
      <WorkLogField
        id="worklog-outcomes"
        label="Results and impact"
        value={outcomes}
        onChange={setOutcomes}
        placeholder="Add measured results where available. Leave blank if not measured yet."
      />
      <WorkLogField
        id="worklog-evidence"
        label="Evidence links"
        value={evidence}
        onChange={setEvidence}
        placeholder="One https:// URL per line — Search Console page, ticket, or published change."
      />
      <WorkLogField
        id="worklog-next"
        label="Next steps"
        value={nextSteps}
        onChange={setNextSteps}
        placeholder="What happens next, who owns it, and when it is due."
      />
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          checked={shareWithClient}
          onChange={(event) => setShareWithClient(event.target.checked)}
          className="mt-1"
        />
        <span>
          Create a public review link after saving. Anyone with the link can
          view this work log; you can revoke it later.
        </span>
      </label>
    </>
  );
}

function WorkLogField({
  id,
  label,
  value,
  onChange,
  placeholder,
  required = false,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  required?: boolean;
}) {
  return (
    <div className="space-y-2">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <Textarea
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        rows={4}
        required={required}
      />
    </div>
  );
}

export function isSafeEvidenceUrl(value: string) {
  try {
    const url = new URL(value);
    return (
      (url.protocol === "https:" || url.protocol === "http:") &&
      !url.username &&
      !url.password &&
      value.length <= 2_048
    );
  } catch {
    return false;
  }
}
