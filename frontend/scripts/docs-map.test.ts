import { readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

// CLAUDE.md's "Docs map" is how an agent finds the one doc it needs instead of
// reading many. This keeps it complete: every Markdown file must be listed.
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

function markdownFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    if (entry.isDirectory()) {
      return skipDirs.has(entry.name)
        ? []
        : markdownFiles(join(dir, entry.name));
    }
    return entry.name.endsWith(".md")
      ? [relative(repoRoot, join(dir, entry.name))]
      : [];
  });
}

describe("CLAUDE.md docs map", () => {
  it("lists every Markdown file in the repository", () => {
    const claude = readFileSync(join(repoRoot, "CLAUDE.md"), "utf8");
    const listed = (path: string) =>
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
});
