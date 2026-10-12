import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MockInstance } from "vitest";
import type { ApiError } from "@/client/lib/uplumoApi";
import {
  archiveProject,
  createProject,
  getProjectAccess,
  getProjects,
  getArchivedProjects,
  restoreProject,
  updateProject,
} from "./projectApi";

const project = {
  id: "p1",
  name: "Acme",
  domain: "acme.com",
  locationCode: 2840,
  languageCode: "en",
  createdAt: "2026-10-12T00:00:00.000Z",
};

let fetchMock: MockInstance<typeof fetch>;

beforeEach(() => {
  fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(Response.json(project));
});

describe("project API contract", () => {
  it("lists projects from the Go endpoint and rejects malformed rows", async () => {
    fetchMock.mockResolvedValue(
      Response.json([project, { ...project, id: 7 }]),
    );

    await expect(getProjects()).rejects.toMatchObject({
      status: 0,
      code: "invalid_response",
    } satisfies Partial<ApiError>);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/projects/list");
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "POST",
      body: "{}",
    });
  });

  it("sends the create payload and returns the server's normalized project", async () => {
    fetchMock.mockResolvedValue(
      Response.json({ ...project, domain: "acme.com" }),
    );

    await expect(
      createProject({ data: { name: "Acme", domain: "https://acme.com" } }),
    ).resolves.toEqual(project);
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/create",
      {
        method: "POST",
        body: JSON.stringify({ name: "Acme", domain: "https://acme.com" }),
      },
    ]);
  });

  it("encodes project IDs in paths and removes the path ID from update JSON", async () => {
    await updateProject({
      data: { projectId: "p/1", name: "Acme", domain: "acme.com" },
    });

    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/update",
      {
        method: "POST",
        body: JSON.stringify({ name: "Acme", domain: "acme.com" }),
      },
    ]);
  });

  it("uses the server's archived-project restore contract", async () => {
    fetchMock.mockResolvedValue(Response.json({ success: true }));

    await expect(
      restoreProject({ data: { archivedProjectId: "old-project" } }),
    ).resolves.toEqual({ success: true });
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/restore",
      {
        method: "POST",
        body: JSON.stringify({ archivedProjectId: "old-project" }),
      },
    ]);
  });

  it("sends an empty archive body and encodes the project path", async () => {
    fetchMock.mockResolvedValue(Response.json({ success: true }));

    await archiveProject({ data: { projectId: "p/1" } });

    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/archive",
      { method: "POST", body: "{}" },
    ]);
  });

  it("uses the active and archived collection endpoints", async () => {
    fetchMock.mockResolvedValue(Response.json([project]));
    await expect(getArchivedProjects()).resolves.toEqual([project]);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/projects/archived");

    fetchMock.mockResolvedValue(Response.json(project));
    await getProjectAccess({ data: { projectId: "p/1" } });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/projects/p%2F1/get");
  });
});
