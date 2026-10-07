import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/seomarineApi";
import {
  billingStatusQueryOptions,
  createCheckout,
  describeStatus,
} from "./planApi";

function respondWith(body: unknown) {
  return vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(body));
}

describe("billing status", () => {
  it.each([
    { plan: "free", status: "none", currentPeriodEnd: null },
    // Go encodes time.Time as RFC 3339, with fractional seconds and an offset.
    {
      plan: "pro",
      status: "active",
      currentPeriodEnd: "2026-11-07T10:15:30.123456+05:30",
    },
  ])("accepts $plan / $status", async (body) => {
    respondWith(body);
    await expect(billingStatusQueryOptions.queryFn()).resolves.toEqual(body);
  });

  it.each([
    { plan: "enterprise", status: "active", currentPeriodEnd: null },
    { plan: "pro", status: "active", currentPeriodEnd: "next month" },
    { plan: "pro", status: "active" },
  ])("rejects %o", async (body) => {
    respondWith(body);
    await expect(billingStatusQueryOptions.queryFn()).rejects.toBeInstanceOf(
      ApiError,
    );
  });
});

describe("createCheckout", () => {
  it("POSTs and rejects a session without a Razorpay key", async () => {
    const fetchMock = respondWith({ subscriptionId: "sub_1", keyId: "" });
    await expect(createCheckout()).rejects.toBeInstanceOf(ApiError);
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/billing/checkout",
      { method: "POST" },
    ]);
  });
});

describe("describeStatus", () => {
  // The Go API sends these exact values; a missing mapping shows raw words.
  it.each([
    ["none", "Free", true],
    ["created", "Awaiting payment", true],
    ["active", "Active", false],
    ["halted", "Payment failed", false],
    ["paused", "Paused", false],
    ["cancelled", "Cancelled", true],
    ["expired", "Expired", true],
  ])("%s reads %s, upgrade offered: %s", (status, label, canUpgrade) => {
    expect(describeStatus(status)).toMatchObject({ label, canUpgrade });
  });

  it("shows an unknown status as sent", () => {
    expect(describeStatus("on_hold")).toEqual({
      label: "on_hold",
      canUpgrade: true,
    });
  });
});
