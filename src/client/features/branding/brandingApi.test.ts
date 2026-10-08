import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/seomarineApi";
import { fetchBranding, resetBranding, saveBranding } from "./brandingApi";

const branding = {
  brandName: "Acme Digital",
  accentColor: "#2563eb",
  logoDataUrl: "data:image/png;base64,iVBORw0KGgo=",
  websiteUrl: "https://acme.agency",
  updatedAt: "2026-10-08T09:00:00.000Z",
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("branding query", () => {
  it("returns the branding row", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json(branding));
    await expect(fetchBranding()).resolves.toEqual(branding);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/branding");
  });

  it("returns null for the default branding", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(null));
    await expect(fetchBranding()).resolves.toBeNull();
  });
});

describe("saveBranding", () => {
  it("posts the input as JSON", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ ok: true }));

    const input = {
      brandName: "Acme Digital",
      accentColor: "#2563eb",
      logoDataUrl: null,
      websiteUrl: null,
    };
    await expect(saveBranding(input)).resolves.toEqual({ ok: true });

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/branding");
    expect(init?.method).toBe("POST");
    expect(init?.headers).toMatchObject({ "Content-Type": "application/json" });
    expect(init?.body).toBe(JSON.stringify(input));
  });

  it("surfaces the server's refusal", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(
        { error: { code: "forbidden", message: "Nope." } },
        { status: 403 },
      ),
    );
    const err = await saveBranding({
      brandName: "Acme",
      accentColor: "#000000",
      logoDataUrl: null,
      websiteUrl: null,
    }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 403, code: "forbidden" });
  });
});

describe("resetBranding", () => {
  it("posts to the reset endpoint", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ ok: true }));
    await expect(resetBranding()).resolves.toEqual({ ok: true });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/branding/reset");
  });
});
