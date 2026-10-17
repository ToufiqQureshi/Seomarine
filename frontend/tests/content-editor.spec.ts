import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from "@playwright/test";

const project = {
  id: "e2e-project",
  name: "Example site",
  domain: "example.com",
  locationCode: 2840,
  languageCode: "en",
  createdAt: "2026-10-17T00:00:00.000Z",
};

const paidEndpoints =
  /content\/brief|assistant\/generate|keywords\/research|domain|backlinks|brand|prompt|rank|local-seo/u;

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function requestBody(route: Route) {
  const body: unknown = route.request().postDataJSON();
  if (!isObjectRecord(body)) throw new Error("Expected a JSON object body");
  return body;
}

async function mockEditorApi(
  page: Page,
  requests: string[],
  writerRequests: Record<string, unknown>[] = [],
) {
  await page.route("**/api/**", async (route: Route) => {
    const pathname = new URL(route.request().url()).pathname;
    requests.push(pathname);
    const now = new Date().toISOString();
    const status = {
      plan: "basic",
      status: "active",
      currentPeriodEnd: null,
      cancelAtPeriodEnd: false,
      unlimited: false,
      paymentsConfigured: true,
      availablePlans: ["basic", "pro", "agency"],
      usage: {
        sites: { used: 1, limit: 1, exceeded: false },
        members: { used: 1, limit: 1, exceeded: false },
        trackedKeywords: { used: 0, limit: 100, exceeded: false },
      },
    };
    const usage = {
      plan: "basic",
      unlimited: false,
      chargesCredits: true,
      balanceCredits: 3900,
      monthlyCredits: 4000,
      periodStart: now,
      periodEnd: null,
      spent: [],
      costs: [{ feature: "ai_writing", unit: "action", credits: 13 }],
    };
    let body: unknown = {};

    if (pathname === "/api/auth/get-session") {
      body = {
        user: {
          id: "e2e-user",
          name: "Editor Test",
          email: "editor@example.com",
          emailVerified: true,
          createdAt: now,
          updatedAt: now,
        },
        session: {
          id: "e2e-session",
          userId: "e2e-user",
          expiresAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
          token: "playwright-session",
          createdAt: now,
          updatedAt: now,
        },
      };
    } else if (pathname.endsWith("/projects/e2e-project/get")) {
      body = project;
    } else if (pathname.endsWith("/content/drafts/list")) {
      body = [];
    } else if (pathname.endsWith("/content/drafts/save")) {
      const input = requestBody(route);
      body = {
        ...input,
        id: typeof input.id === "string" ? input.id : "e2e-draft",
        projectId: project.id,
        createdAt: now,
        updatedAt: now,
      };
    } else if (pathname === "/api/v1/config/content-writer-status") {
      body = {
        configured: true,
        message: "",
        maxCreditsPerAction: 15,
      };
    } else if (pathname.endsWith("/content/assistant/generate")) {
      await new Promise((resolve) => setTimeout(resolve, 300));
      writerRequests.push(requestBody(route));
      body = { content: "## A useful outline" };
    } else if (pathname === "/api/v1/billing/usage") {
      body = usage;
    } else if (pathname === "/api/v1/billing/status") {
      body = status;
    } else if (pathname === "/api/v1/billing/plans") {
      body = { plans: [] };
    }

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
}

async function openEditor(
  page: Page,
  requests: string[],
  writerRequests: Record<string, unknown>[] = [],
) {
  await mockEditorApi(page, requests, writerRequests);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(`/p/${project.id}/content-editor`);
  await expect(
    page.getByRole("heading", { name: "Content Editor" }),
  ).toBeVisible();
  await expect(page.getByLabel(/Article title/)).toBeVisible();
  await expect(page.getByText("Your article", { exact: true })).toBeVisible();
}

test("a rapid AI writer double click sends one keyed request", async ({
  page,
}) => {
  const requests: string[] = [];
  const writerRequests: Record<string, unknown>[] = [];
  await openEditor(page, requests, writerRequests);
  await page.getByRole("tab", { name: "AI help" }).click();
  await expect(page.getByText("Up to 15 credits")).toHaveCount(10);
  const outline = page.getByRole("button", { name: "Create outline" });
  await expect(outline).toBeEnabled();
  await outline.dispatchEvent("click");
  await outline.dispatchEvent("click");
  await expect.poll(() => writerRequests.length).toBe(1);
  expect(writerRequests[0]?.idempotencyKey).toMatch(/^[0-9a-f-]{36}$/iu);
});

test("block insert menu stays beside the editor and inside the viewport", async ({
  page,
}, testInfo: TestInfo) => {
  const requests: string[] = [];
  await openEditor(page, requests);
  await page.getByText("How it looks on Google").click();

  const article = page.locator(
    '[role="textbox"][aria-labelledby="article-editor-label"]',
  );
  await article.scrollIntoViewIfNeeded();
  await article.click();
  await page.keyboard.press("/");
  const menu = page.getByRole("menu", { name: "Add a block" });
  await expect(menu).toBeVisible();

  const menuBox = await menu.boundingBox();
  const fields = await Promise.all(
    [
      "#content-draft-title",
      "#content-draft-keyword",
      "#content-draft-meta",
    ].map((selector) => page.locator(selector).boundingBox()),
  );
  expect(menuBox).not.toBeNull();
  if (!menuBox) throw new Error("Block menu has no visible bounds.");
  expect(menuBox.x).toBeGreaterThanOrEqual(0);
  expect(menuBox.y).toBeGreaterThanOrEqual(0);
  expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(1280);
  expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(900);
  for (const field of fields) {
    if (!field) continue;
    const overlaps =
      menuBox.x < field.x + field.width &&
      menuBox.x + menuBox.width > field.x &&
      menuBox.y < field.y + field.height &&
      menuBox.y + menuBox.height > field.y;
    expect(overlaps).toBe(false);
  }

  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await article.click();
  await page.keyboard.press("/");
  await expect(menu).toBeVisible();
  await page.getByRole("heading", { name: "Content Editor" }).click();
  await expect(menu).toBeHidden();

  await testInfo.attach("content-editor-block-menu.png", {
    body: await page.screenshot(),
    contentType: "image/png",
  });
});

test("page load, 30 seconds of typing, and autosave trigger no paid lookups", async ({
  page,
}) => {
  test.setTimeout(60_000);
  const requests: string[] = [];
  await openEditor(page, requests);
  const title = page.getByLabel(/Article title/);
  for (const letter of "A practical SEO guide for small teams") {
    await title.pressSequentially(letter, { delay: 1000 });
  }
  await expect(page.getByText(/· Saved$/u)).toBeVisible({ timeout: 10_000 });

  expect(requests.filter((path) => paidEndpoints.test(path))).toEqual([]);
  expect(
    requests.filter((path) => path.endsWith("/content/drafts/save")),
  ).toHaveLength(1);
});
