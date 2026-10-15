import { readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

// Repo-wide docs are listed in CLAUDE.md. Feature docs are colocated with code
// so an agent can find the local README/flow/decision set without a long index.
const repoRoot = resolve(import.meta.dirname, "../..");
const skipDirs = new Set([
  "node_modules",
  ".git",
  "dist",
  "skills",
  "testdata",
]);
const frontendFeatures = "frontend/src/client/features/";
// Blog posts are site content, not docs; only the folder's README is listed.
const blogPosts = "backend/internal/site/blog/";
const featureDocNames = new Set(["README.md", "flow.md", "decision.md"]);

function isColocatedFeatureDoc(path: string): boolean {
  const name = path.slice(path.lastIndexOf("/") + 1);
  return (
    featureDocNames.has(name) &&
    (/^frontend\/src\/client\/features\/[^/]+\/(README|flow|decision)\.md$/.test(
      path,
    ) ||
      /^backend\/internal\/.+\/(README|flow|decision)\.md$/.test(path))
  );
}

function goPackageDirs(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    if (!entry.isDirectory() || skipDirs.has(entry.name)) return [];
    const child = join(dir, entry.name);
    const files = readdirSync(child, { withFileTypes: true });
    return [
      ...(files.some((file) => file.isFile() && file.name.endsWith(".go"))
        ? [child]
        : []),
      ...goPackageDirs(child),
    ];
  });
}

function markdownFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    if (entry.isDirectory()) {
      return skipDirs.has(entry.name)
        ? []
        : markdownFiles(join(dir, entry.name));
    }
    return entry.name.endsWith(".md")
      ? [relative(repoRoot, join(dir, entry.name)).replaceAll("\\", "/")]
      : [];
  });
}

describe("CLAUDE.md docs map", () => {
  it("lists every Markdown file in the repository", () => {
    const claude = readFileSync(join(repoRoot, "CLAUDE.md"), "utf8");
    const listed = (path: string) =>
      isColocatedFeatureDoc(path) ||
      (path.startsWith(blogPosts) && !path.endsWith("/README.md")) ||
      claude.includes(`\`${path}\``) ||
      (path.startsWith("docs/audits/") && claude.includes("`docs/audits/`")) ||
      (path.startsWith(frontendFeatures) &&
        claude.includes(`\`${path.slice(frontendFeatures.length)}\``));

    const missing = markdownFiles(repoRoot).filter(
      (path) => path !== "CLAUDE.md" && !listed(path),
    );
    expect(missing, "add these to the Docs map in CLAUDE.md").toEqual([]);
  });

  it("keeps the three internal docs beside every frontend feature", () => {
    const featureRoot = join(repoRoot, frontendFeatures);
    const missing = readdirSync(featureRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .flatMap((entry) =>
        [...featureDocNames]
          .filter(
            (name) =>
              !markdownFiles(join(featureRoot, entry.name)).includes(
                `${frontendFeatures}${entry.name}/${name}`,
              ),
          )
          .map((name) => `${entry.name}/${name}`),
      );
    expect(
      missing,
      "create README.md, flow.md and decision.md per feature",
    ).toEqual([]);
  });

  it("keeps the three internal docs beside every Go package", () => {
    const missing = goPackageDirs(join(repoRoot, "backend/internal")).flatMap(
      (dir) =>
        [...featureDocNames]
          .filter(
            (name) =>
              !markdownFiles(dir).includes(
                `${relative(repoRoot, dir)}/${name}`,
              ),
          )
          .map((name) => `${relative(repoRoot, dir)}/${name}`),
    );
    expect(
      missing,
      "create README.md, flow.md and decision.md per Go package",
    ).toEqual([]);
  });
});
