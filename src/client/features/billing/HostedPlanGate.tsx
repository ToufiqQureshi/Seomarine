import { useCustomer } from "autumn-js/react";
import { useSession } from "@/lib/auth-client";
import { isHostedClientAuthMode } from "@/lib/auth-mode";
import {
  getCustomerPlanStatus,
  type PlanStatus,
} from "@/client/features/billing/plan-detection";

// The single client-side plan gate. It is a UX layer only: the server enforces
// every paid-plan limit before it spends anything.
export function useHostedPlanGate(): "loading" | PlanStatus {
  // Self-hosted has no Autumn customer and resolves to the paid tier on the
  // server, so only hosted mode looks up the plan.
  const isHostedMode = isHostedClientAuthMode();
  const { data: session, isPending: isSessionPending } = useSession();
  const hasSession = Boolean(session?.user?.id);
  const customerQuery = useCustomer({
    queryOptions: { enabled: isHostedMode && hasSession },
  });

  if (!isHostedMode) return "paid";
  if (isSessionPending || !hasSession || customerQuery.isLoading) {
    return "loading";
  }
  // Fails closed: a customer that failed to load resolves to "free".
  return getCustomerPlanStatus(customerQuery.data);
}
