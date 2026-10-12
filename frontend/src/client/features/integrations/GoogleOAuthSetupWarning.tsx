import { AlertTriangle } from "lucide-react";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/client/components/ui/alert";

export function GoogleOAuthSetupWarning({
  integrationName,
}: {
  integrationName: string;
}) {
  return (
    <Alert variant="warning">
      <AlertTriangle className="size-4" />
      <AlertTitle>Google connection unavailable</AlertTitle>
      <AlertDescription>
        <p>
          Connecting {integrationName} isn&apos;t available right now. Please
          try again later.
        </p>
      </AlertDescription>
    </Alert>
  );
}
