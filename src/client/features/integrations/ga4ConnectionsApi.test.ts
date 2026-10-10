import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiRequest } from "@/client/lib/seomarineApi";
import {
  disconnectGa4,
  ga4ConnectionSchema,
  getGa4Connection,
  listGa4Properties,
  setGa4Property,
} from "./ga4ConnectionsApi";

vi.mock("@/client/lib/seomarineApi", () => ({ apiRequest: vi.fn() }));

const request = vi.mocked(apiRequest);

beforeEach(() => request.mockReset());

describe("GA4 connection API", () => {
  it("uses encoded project paths and validates nullable connection metadata", async () => {
    const response = {
      connected: false,
      canManage: true,
      currentUserHasGrant: false,
      googleOAuthConfigured: true,
      propertyId: null,
      propertyDisplayName: null,
      propertyTimeZone: null,
      propertyCurrencyCode: null,
      connectedByEmail: null,
      connectedAt: null,
    };
    request.mockResolvedValue(response);

    await getGa4Connection("project/one");

    expect(request).toHaveBeenCalledWith(
      "/api/v1/projects/project%2Fone/ga4/connection/status",
      expect.any(Object),
      "POST",
      {},
    );
    expect(ga4ConnectionSchema.safeParse(response).success).toBe(true);
    expect(
      ga4ConnectionSchema.safeParse({ ...response, connected: "false" })
        .success,
    ).toBe(false);
  });

  it("lists properties from the Go API and submits only the selected account/property", async () => {
    request.mockResolvedValueOnce({ accounts: [] }).mockResolvedValueOnce({
      connected: true,
      propertyId: "properties/123",
      propertyDisplayName: "Example",
      propertyTimeZone: "UTC",
      propertyCurrencyCode: "USD",
      connectedByEmail: null,
      connectedAt: "2026-10-10T00:00:00.000Z",
    });

    await listGa4Properties("project-a");
    await setGa4Property("project-a", {
      accountId: "account-a",
      propertyId: "properties/123",
    });

    expect(request.mock.calls[0]?.[0]).toBe(
      "/api/v1/projects/project-a/ga4/properties/list",
    );
    expect(request.mock.calls[0]?.[3]).toEqual({});
    expect(request.mock.calls[1]?.[0]).toBe(
      "/api/v1/projects/project-a/ga4/connection/set",
    );
    expect(request.mock.calls[1]?.[3]).toEqual({
      accountId: "account-a",
      propertyId: "properties/123",
    });
  });

  it("disconnects through the project-scoped Go endpoint", async () => {
    request.mockResolvedValue({ connected: false });

    await disconnectGa4("project-a");

    expect(request).toHaveBeenCalledWith(
      "/api/v1/projects/project-a/ga4/connection/disconnect",
      expect.any(Object),
      "POST",
      {},
    );
  });
});
