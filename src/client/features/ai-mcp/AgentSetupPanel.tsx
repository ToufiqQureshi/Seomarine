import { Package } from "lucide-react";
import { CopyButton } from "@/client/components/CopyButton";

export const AGENT_SETUP_DESCRIPTION =
  "Paste this prompt into your agent and it will connect Seomarine for you.";

export function AgentSetupPanel({
  prompt,
  onCopy,
}: {
  prompt: string;
  onCopy?: () => void;
}) {
  return (
    <div className="rounded-xl border border-border bg-background/25 p-5">
      <div className="mb-5 flex items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-card">
          <Package className="size-5 text-muted-foreground" />
        </span>
        <div>
          <p className="text-sm font-medium">Seomarine for AI agents</p>
          <p className="mt-1 text-xs text-muted-foreground">
            Connects your agent to your Seomarine data
          </p>
        </div>
      </div>
      <CopyButton
        variant="default"
        size="lg"
        className="w-full"
        value={prompt}
        label="Copy setup prompt"
        successMessage="Setup prompt copied"
        onCopy={onCopy}
      />
    </div>
  );
}
