import { ShieldAlert, Wrench } from "lucide-react";
import { GateCard } from "@/client/components/GateCard";
import { Alert, AlertDescription } from "@/client/components/ui/alert";
import { Button } from "@/client/components/ui/button";

// Shown when the server cannot run SAM right now. The model key belongs to
// Seomarine, so there is nothing for the customer to configure: only wait and
// retry.
export function SamSetupGate({
  errorMessage,
  isRefetching,
  onRetry,
}: {
  errorMessage: string | null;
  isRefetching: boolean;
  onRetry: () => void;
}) {
  return (
    <GateCard
      icon={Wrench}
      tone="warning"
      title="SAM is unavailable right now"
      description={
        <p>
          SAM, Seomarine&apos;s in-app AI agent, can&apos;t start at the moment.
          Try again in a few minutes.
        </p>
      }
      actions={
        <Button size="lg" pending={isRefetching} onClick={onRetry}>
          Try again
        </Button>
      }
    >
      {errorMessage ? (
        <Alert variant="warning">
          <ShieldAlert />
          <AlertDescription className="text-foreground">
            {errorMessage}
          </AlertDescription>
        </Alert>
      ) : null}
    </GateCard>
  );
}
