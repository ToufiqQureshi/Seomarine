import { createServerFn } from "@tanstack/react-start";
import { APIError } from "better-auth/api";
import { getRequest } from "@tanstack/react-start/server";
import { z } from "zod";
import { getAuth, getHostedBaseUrl } from "@/lib/auth";
import { hasOrgPermission } from "@/lib/org-permissions";
import { consumeInvitationSendBudget } from "@/server/auth/invitation-send-limit";
import { requireOrgPermission } from "@/server/auth/org-gate";
import { AuthRepository } from "@/server/auth/repositories/AuthRepository";
import { sendHostedInvitationEmail } from "@/server/email/loops";
import { AppError } from "@/server/lib/errors";
import { requireAuthenticatedContext } from "@/serverFunctions/middleware";

const sendInvitationSchema = z.object({ email: z.string().email() });

// Invite (or re-invite) a teammate. better-auth creates the pending
// invitation — its server-side checks (inviter must be a member with invite
// permission, admin-only role lock, 20-pending cap) all still run — but the
// email is sent here rather than via the plugin's sendInvitationEmail
// callback: better-auth swallows throws from that callback, so a failed send
// would still read as "sent". Here it fails the call. resend: true re-mails
// an existing pending invitation with the same link and a refreshed expiry.
export const sendTeamInvitation = createServerFn({ method: "POST" })
  .middleware(requireAuthenticatedContext)
  .validator(sendInvitationSchema)
  .handler(async ({ data, context }) => {
    // Defense in depth: fail-closed locally before touching the rate-limit
    // budget or calling the plugin, so the budget cannot be exhausted by
    // low-privilege callers.
    requireOrgPermission(context, { invitation: ["create"] });

    await consumeInvitationSendBudget(context.organizationId, data.email);

    const invitation = await getAuth()
      .api.createInvitation({
        headers: getRequest().headers,
        body: {
          email: data.email,
          role: "admin",
          resend: true,
          // Bind the invitation to the request's resolved org, not the session's
          // active-organization hint, which can be stale after a switch.
          organizationId: context.organizationId,
        },
      })
      .catch((error: unknown) => {
        // The invite dialog shows this one inline, next to the email.
        if (
          error instanceof APIError &&
          error.body?.code === "USER_IS_ALREADY_A_MEMBER_OF_THIS_ORGANIZATION"
        ) {
          throw new AppError("CONFLICT");
        }
        throw error;
      });

    const [inviter, memberships] = await Promise.all([
      AuthRepository.getHostedUser(context.userId),
      AuthRepository.listMembershipsForUser(context.userId),
    ]);
    const organizationName =
      memberships.find(
        (membership) => membership.organizationId === context.organizationId,
      )?.organizationName ?? "Organization";

    try {
      await sendHostedInvitationEmail({
        email: data.email,
        inviteUrl: `${getHostedBaseUrl()}/accept-invitation/${invitation.id}`,
        organizationName,
        inviterName: inviter?.name?.trim() || context.userEmail,
        inviterEmail: context.userEmail,
      });
    } catch (error) {
      // The invitation row exists and stays pending — surface the send
      // failure so the inviter retries instead of assuming it landed.
      console.error("Invitation email send failed:", error);
      throw new AppError("UPSTREAM_UNAVAILABLE");
    }

    return { invitationId: invitation.id };
  });
