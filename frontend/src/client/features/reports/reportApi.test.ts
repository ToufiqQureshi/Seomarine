import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/uplumoApi";
import {
  deleteReport,
  getReport,
  listReports,
  saveReport,
  saveReportTemplate,
  shareReport,
  unshareReport,
} from "./reportApi";

afterEach(() => vi.restoreAllMocks());

const report = {
  id: "r1",
  projectId: "p1",
  title: "Weekly report",
  summary: "Summary",
  skill: null,
  templateId: "t1",
  createdBy: "owner@example.com",
  createdByUserId: "u1",
  sizeBytes: 200,
  shareToken: null,
  sharedAt: null,
  createdAt: "2026-10-12T10:00:00Z",
  updatedAt: "2026-10-12T10:00:00Z",
};
const template = {
  id: "t1",
  projectId: "p1",
  name: "Weekly",
  description: "Weekly summary",
  instructions: "Summarize the week",
  createdBy: "owner@example.com",
  createdByUserId: "u1",
  createdAt: "2026-10-12T10:00:00Z",
  updatedAt: "2026-10-12T10:00:00Z",
};

describe("reports API", () => {
  it("enriches report listings and caps page size", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(Response.json({ reports: [report] }))
      .mockResolvedValueOnce(
        Response.json({ templates: [template], remaining: 3 }),
      );
    await expect(
      listReports({ data: { projectId: "p/1", limit: 999, offset: 10 } }),
    ).resolves.toEqual({
      reports: [{ ...report, createdByName: null, templateName: "Weekly" }],
    });
    expect(fetchMock.mock.calls).toMatchObject([
      [
        "/api/v1/projects/p%2F1/reports/list",
        { body: JSON.stringify({ limit: 50, offset: 10 }) },
      ],
      ["/api/v1/projects/p%2F1/reports/templates/list", { body: "{}" }],
    ]);
  });

  it("reports missing templates as null and validates share/delete responses", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        Response.json({ report: { ...report, templateId: "missing" } }),
      )
      .mockResolvedValueOnce(Response.json({ templates: [], remaining: 5 }))
      .mockResolvedValueOnce(
        Response.json({
          reportId: "r1",
          public: true,
          shareToken: "token",
          sharedAt: "now",
        }),
      )
      .mockResolvedValueOnce(
        Response.json({
          reportId: "r1",
          public: false,
          shareToken: null,
          sharedAt: null,
        }),
      )
      .mockResolvedValueOnce(Response.json({ reportId: "r1" }));
    await expect(
      getReport({ data: { projectId: "p1", reportId: "r1" } }),
    ).resolves.toMatchObject({ templateName: null });
    await expect(
      shareReport({ data: { projectId: "p1", reportId: "r1" } }),
    ).resolves.toMatchObject({ public: true });
    await expect(
      unshareReport({ data: { projectId: "p1", reportId: "r1" } }),
    ).resolves.toMatchObject({ public: false });
    await expect(
      deleteReport({ data: { projectId: "p1", reportId: "r1" } }),
    ).resolves.toEqual({ reportId: "r1" });
    expect(
      fetchMock.mock.calls.slice(2).map(([path, init]) => [path, init?.body]),
    ).toEqual([
      [
        "/api/v1/projects/p1/reports/sharing",
        JSON.stringify({ reportId: "r1", public: true }),
      ],
      [
        "/api/v1/projects/p1/reports/sharing",
        JSON.stringify({ reportId: "r1", public: false }),
      ],
      [
        "/api/v1/projects/p1/reports/delete",
        JSON.stringify({ reportId: "r1" }),
      ],
    ]);
  });

  it("maps known template validation errors but rethrows other failures", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      Response.json(
        {
          error: {
            code: "VALIDATION_ERROR",
            message: "Template limit reached",
          },
        },
        { status: 400 },
      ),
    );
    await expect(
      saveReportTemplate({
        data: {
          projectId: "p1",
          name: "New",
          description: "desc",
          instructions: "instructions",
        },
      }),
    ).resolves.toEqual({ ok: false, message: "Template limit reached" });
    fetchMock.mockResolvedValueOnce(
      Response.json(
        { error: { code: "internal", message: "Try later" } },
        { status: 500 },
      ),
    );
    await expect(
      saveReportTemplate({
        data: {
          projectId: "p1",
          name: "New",
          description: "desc",
          instructions: "instructions",
        },
      }),
    ).rejects.toBeInstanceOf(ApiError);
  });

  it("saves a manually authored report through the project-scoped endpoint", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      Response.json({
        reportId: "r2",
        title: "Monthly update",
        created: true,
        htmlBytes: 240,
      }),
    );

    await expect(
      saveReport({
        data: {
          projectId: "p/1",
          title: "Monthly update",
          summary: "A concise update",
          html: "<!doctype html><html></html>",
        },
      }),
    ).resolves.toMatchObject({ reportId: "r2", created: true });

    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/reports/save",
      {
        body: JSON.stringify({
          reportId: undefined,
          title: "Monthly update",
          summary: "A concise update",
          html: "<!doctype html><html></html>",
          skill: "Manual",
        }),
      },
    ]);
  });
});
