import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/uplumoApi";
import {
  billingPlansQueryOptions,
  billingStatusQueryOptions,
  cancelSubscription,
  createCheckout,
  describeStatus,
} from "./planApi";

function respondWith(body: unknown) {
  return vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(body));
}

describe("billing status", () => {
  it.each([
    {
      plan: "free",
      status: "none",
      currentPeriodEnd: null,
      cancelAtPeriodEnd: false,
      unlimited: false,
      paymentsConfigured: false,
      availablePlans: [],
    },
    // Go encodes time.Time as RFC 3339, with fractional seconds and an offset.
    {
      plan: "pro",
      status: "active",
      currentPeriodEnd: "2026-11-07T10:15:30.123456+05:30",
      cancelAtPeriodEnd: true,
      unlimited: false,
      paymentsConfigured: true,
      availablePlans: ["pro"],
    },
  ])("accepts $plan / $status", async (body) => {
    respondWith(body);
    await expect(billingStatusQueryOptions.queryFn()).resolves.toEqual(body);
  });

  it.each([
    { plan: "enterprise", status: "active", currentPeriodEnd: null },
    { plan: "basic", status: "active", currentPeriodEnd: "next month" },
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
    await expect(createCheckout("pro")).rejects.toBeInstanceOf(ApiError);
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/billing/checkout",
      { method: "POST", body: JSON.stringify({ plan: "pro" }) },
    ]);
  });
});

describe("cancelSubscription", () => {
  it("POSTs and returns the plan that results", async () => {
    const body = {
      plan: "pro",
      status: "active",
      currentPeriodEnd: "2026-11-07T10:15:30Z",
      cancelAtPeriodEnd: true,
      unlimited: false,
      paymentsConfigured: true,
      availablePlans: ["pro"],
    };
    const fetchMock = respondWith(body);
    await expect(cancelSubscription()).resolves.toEqual(body);
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/billing/cancel",
      { method: "POST" },
    ]);
  });

  it("rejects an answer that does not say whether Pro was cancelled", async () => {
    respondWith({ plan: "pro", status: "active", currentPeriodEnd: null });
    await expect(cancelSubscription()).rejects.toBeInstanceOf(ApiError);
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

describe("billing plans", () => {
  const plan = {
    id: "basic",
    name: "Basic",
    priceInr: 999,
    summary: "For one site.",
    highlights: ["1 website"],
  };

  it("returns the catalog the Go API sends", async () => {
    const fetchMock = respondWith({ plans: [plan] });
    await expect(billingPlansQueryOptions.queryFn()).resolves.toEqual([plan]);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/billing/plans");
  });

  it.each([
    { plans: [{ ...plan, id: "enterprise" }] },
    { plans: [{ ...plan, priceInr: "999" }] },
    { plans: [{ ...plan, priceInr: -1 }] },
    {},
  ])("rejects a catalog it cannot trust: %j", async (body) => {
    respondWith(body);
    await expect(billingPlansQueryOptions.queryFn()).rejects.toBeInstanceOf(
      ApiError,
    );
  });
});
