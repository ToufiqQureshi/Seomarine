import { afterEach, describe, expect, it, vi } from "vitest";
import { tableQueryOptions } from "./searchPerformanceQueries";

afterEach(() => vi.restoreAllMocks());

describe("search performance table query", () => {
  it("uses the same project, pagination, and filters in its key and request", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ connected: false }));
    const options = tableQueryOptions("project/1", "query", 2, 50, {
      dateRange: "last_3_months",
      device: "MOBILE",
      country: "usa",
      queryFilter: { operator: "contains", expression: "dentist" },
    });
    expect(options.queryKey).toEqual([
      "searchPerformanceTable",
      "project/1",
      "query",
      2,
      50,
      {
        dateRange: "last_3_months",
        device: "MOBILE",
        country: "usa",
        queryFilter: { operator: "contains", expression: "dentist" },
      },
    ]);
    await options.queryFn?.({} as never);
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe(
      "/api/v1/projects/project%2F1/gsc/search-performance/table",
    );
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      queryFilter: { operator: "contains", expression: "dentist" },
      dateRange: "last_3_months",
      device: "MOBILE",
      country: "usa",
      dimension: "query",
      page: 2,
      pageSize: 50,
    });
  });
});
