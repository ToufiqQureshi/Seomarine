import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check } from "lucide-react";
import { toast } from "sonner";
import { ErrorState } from "@/client/components/ErrorState";
import { PageHeader } from "@/client/components/PageHeader";
import { QueryError } from "@/client/components/QueryState";
import { SkeletonCard } from "@/client/components/SkeletonPresets";
import { Badge } from "@/client/components/ui/badge";
import { Button } from "@/client/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import { getStandardErrorMessage } from "@/client/lib/error-messages";
import { ApiError } from "@/client/lib/seomarineApi";
import {
  billingStatusQueryOptions,
  createCheckout,
  describeStatus,
  type BillingStatus,
} from "./planApi";
import { openRazorpayCheckout } from "./razorpay";

// Owner decision pending (ROADMAP open decisions): keep in sync with the
// amount on the Razorpay plan the Go API subscribes customers to.
const PRO_PRICE_INR = 1999;

const PRO_FEATURES = [
  "AI traffic analytics: visitors from ChatGPT, Perplexity, Gemini, Claude and Copilot",
  "AI visibility included, no extra add-on",
  "No row limits on your own data",
  "Agency-branded client reports",
];

const inr = new Intl.NumberFormat("en-IN", {
  style: "currency",
  currency: "INR",
  maximumFractionDigits: 0,
});

export function BillingPlanPage() {
  const statusQuery = useQuery(billingStatusQueryOptions);

  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-3xl space-y-6">
        <PageHeader
          title="Billing"
          description="Your plan, what it costs and when it renews."
        />
        {statusQuery.isPending ? (
          <SkeletonCard />
        ) : statusQuery.isError ? (
          <StatusError
            error={statusQuery.error}
            onRetry={() => void statusQuery.refetch()}
            isRetrying={statusQuery.isFetching}
          />
        ) : (
          <PlanCard status={statusQuery.data} />
        )}
      </div>
    </div>
  );
}

function StatusError({
  error,
  onRetry,
  isRetrying,
}: {
  error: unknown;
  onRetry: () => void;
  isRetrying: boolean;
}) {
  // A retry can't fix an ended session: send the owner to sign in instead.
  if (error instanceof ApiError && error.status === 401) {
    return (
      <ErrorState
        message={error.message}
        action={
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={<Link to="/sign-in" />}
          >
            Sign in
          </Button>
        }
      />
    );
  }
  return (
    <QueryError
      error={error}
      fallback="We couldn't load your plan."
      onRetry={onRetry}
      isRetrying={isRetrying}
    />
  );
}

function PlanCard({ status }: { status: BillingStatus }) {
  const isPro = status.plan === "pro";
  const { label, detail, canUpgrade } = describeStatus(status.status);
  const renews = status.currentPeriodEnd
    ? new Date(status.currentPeriodEnd).toLocaleDateString("en-IN", {
        day: "numeric",
        month: "long",
        year: "numeric",
      })
    : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          {isPro ? "Seomarine Pro" : "Free plan"}
          <Badge variant={status.status === "active" ? "success" : "soft"}>
            {label}
          </Badge>
        </CardTitle>
        <CardDescription>
          {isPro
            ? renews
              ? `Renews on ${renews}.`
              : "Your Pro plan is set up."
            : `Upgrade to Pro for ${inr.format(PRO_PRICE_INR)} a month, taxes included.`}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {detail ? <p className="text-sm">{detail}</p> : null}
        <ul className="space-y-2 text-sm">
          {PRO_FEATURES.map((feature) => (
            <li key={feature} className="flex gap-2">
              <Check className="mt-0.5 size-4 shrink-0 text-primary" />
              {feature}
            </li>
          ))}
        </ul>
      </CardContent>
      {isPro || !canUpgrade ? null : (
        <CardFooter>
          <UpgradeButton />
        </CardFooter>
      )}
    </Card>
  );
}

function UpgradeButton() {
  const queryClient = useQueryClient();
  const upgrade = useMutation({
    mutationFn: async () => openRazorpayCheckout(await createCheckout()),
    meta: { errorToast: false },
    onSuccess: async (paid) => {
      if (!paid) return;
      toast.success("Payment received. Welcome to Seomarine Pro!");
      // Razorpay tells our server by webhook, so the plan can lag a moment.
      await queryClient.invalidateQueries({
        queryKey: billingStatusQueryOptions.queryKey,
      });
    },
  });

  return (
    <div className="w-full space-y-3">
      <Button onClick={() => upgrade.mutate()} disabled={upgrade.isPending}>
        {upgrade.isPending
          ? "Opening checkout…"
          : `Upgrade for ${inr.format(PRO_PRICE_INR)}/month`}
      </Button>
      {upgrade.isError ? (
        <ErrorState
          variant="inline"
          message={getStandardErrorMessage(upgrade.error)}
        />
      ) : null}
    </div>
  );
}
