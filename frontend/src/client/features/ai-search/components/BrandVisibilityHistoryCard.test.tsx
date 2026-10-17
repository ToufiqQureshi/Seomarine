import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { BrandVisibilityHistoryCard } from "./BrandVisibilityHistoryCard";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: React.ReactNode }) => children,
}));

describe("BrandVisibilityHistoryCard", () => {
  it("renders the empty state without reading a missing period", () => {
    const client = new QueryClient();
    const html = renderToStaticMarkup(
      <QueryClientProvider client={client}>
        <BrandVisibilityHistoryCard
          projectId="project-1"
          snapshots={[]}
          isLoading={false}
          isError={false}
        />
      </QueryClientProvider>,
    );

    expect(html).toContain("No monthly snapshots yet");
  });
});
