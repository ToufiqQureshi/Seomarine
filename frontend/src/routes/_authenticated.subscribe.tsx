import { createFileRoute, redirect } from "@tanstack/react-router";
import { normalizeAuthRedirect } from "@/lib/auth-redirect";

export const Route = createFileRoute("/_authenticated/subscribe")({
  validateSearch: (
    search: Record<string, unknown>,
  ): { upgrade?: true; redirect?: string; checkout?: "success" } => ({
    upgrade:
      search.upgrade === true || search.upgrade === "true" ? true : undefined,
    redirect:
      typeof search.redirect === "string"
        ? normalizeAuthRedirect(search.redirect)
        : undefined,
    checkout: search.checkout === "success" ? "success" : undefined,
  }),
  beforeLoad: () => {
    throw redirect({ to: "/billing/plan", replace: true });
  },
  component: () => null,
});
