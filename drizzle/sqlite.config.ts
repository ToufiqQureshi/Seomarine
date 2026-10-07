import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { defineConfig } from "drizzle-kit";

const wranglerDir = path.resolve(".wrangler");

function findLocalD1File(): string | undefined {
  return fs
    .readdirSync(wranglerDir, { encoding: "utf-8", recursive: true })
    .find((file) => file.endsWith(".sqlite"));
}

// The local D1 database `wrangler dev` keeps under .wrangler/. Without
// .wrangler (CI, fresh clones) there is no local database to point at.
function getLocalD1Url(): string {
  if (!fs.existsSync(wranglerDir)) return "";
  let file = findLocalD1File();
  if (!file) {
    // "DB" is the D1 binding in wrangler.jsonc; any query creates the file.
    execSync('pnpm exec wrangler d1 execute DB --local --command "SELECT 1;"', {
      stdio: "pipe",
    });
    file = findLocalD1File();
    if (!file) throw new Error("wrangler did not create the local D1 file");
  }
  return path.resolve(wranglerDir, file);
}

export default defineConfig({
  dialect: "sqlite",
  // The raw SQLite barrel (not ../schema, the provider-aware one, which imports
  // cloudflare:workers and can't load under drizzle-kit's node runtime).
  schema: "./src/db/d1/schema.ts",
  out: "./drizzle/sqlite",
  dbCredentials: { url: getLocalD1Url() },
});
