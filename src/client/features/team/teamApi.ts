import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

const memberSchema = z.object({
  id: z.string(),
  userId: z.string(),
  role: z.string(),
  user: z.object({ name: z.string().nullable(), email: z.string() }),
});

const invitationSchema = z.object({
  id: z.string(),
  email: z.string(),
  role: z.string().nullable(),
  expiresAt: z.string(),
});

export const teamSchema = z.object({
  members: z.array(memberSchema),
  pendingInvitations: z.array(invitationSchema),
});

export function getTeam() {
  return apiRequest("/api/v1/organization/team", teamSchema, "GET");
}

export function transferOwnership({ data }: { data: { memberId: string } }) {
  return apiRequest(
    "/api/v1/organization/ownership/transfer",
    z.object({ memberId: z.string() }),
    "POST",
    data,
  );
}
