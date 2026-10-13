import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/uplumoApi";
import { getTeam, sendTeamInvitation, transferOwnership } from "./teamApi";
import { getOrganizationContext, switchOrganization } from "./organizationApi";

afterEach(() => vi.restoreAllMocks());

describe("team and organization API", () => {
  it("validates team members and pending invitations", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        members: [
          {
            id: "m1",
            userId: "u1",
            role: "owner",
            user: { name: null, email: "owner@example.com" },
          },
        ],
        pendingInvitations: [
          {
            id: "i1",
            email: "invitee@example.com",
            role: "admin",
            expiresAt: "2026-10-19T00:00:00Z",
          },
        ],
      }),
    );
    await expect(getTeam()).resolves.toMatchObject({
      members: [{ user: { name: null } }],
      pendingInvitations: [{ id: "i1" }],
    });
    fetchMock.mockResolvedValueOnce(
      Response.json({ members: [{ id: "m1" }], pendingInvitations: [] }),
    );
    await expect(getTeam()).rejects.toBeInstanceOf(ApiError);
  });

  it("sends team changes to their organization endpoints", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(Response.json({ invitationId: "i1" }))
      .mockResolvedValueOnce(Response.json({ memberId: "m1" }))
      .mockResolvedValueOnce(
        Response.json({
          organizationId: "o2",
          organizationName: "Other",
          role: "admin",
          organizations: [],
        }),
      )
      .mockResolvedValueOnce(Response.json({ organizationId: "o2" }));
    await expect(
      sendTeamInvitation({ data: { email: "person@example.com" } }),
    ).resolves.toEqual({ invitationId: "i1" });
    await expect(
      transferOwnership({ data: { memberId: "m1" } }),
    ).resolves.toEqual({ memberId: "m1" });
    await expect(getOrganizationContext()).resolves.toMatchObject({
      organizationId: "o2",
    });
    await expect(
      switchOrganization({ data: { organizationId: "o2" } }),
    ).resolves.toEqual({ organizationId: "o2" });
    expect(fetchMock.mock.calls.map(([path]) => path)).toEqual([
      "/api/v1/organization/invitations",
      "/api/v1/organization/ownership/transfer",
      "/api/v1/organization/context",
      "/api/v1/organization/switch",
    ]);
  });
});
