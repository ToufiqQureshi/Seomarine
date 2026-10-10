import { spawn } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import postgres from "postgres";

const PAGE_SIZE = 250;
const IDENTIFIER = /^[A-Za-z_][A-Za-z0-9_]*$/;
const INTERNAL_TABLES = new Set([
  "d1_migrations",
  "sqlite_sequence",
  "sqlite_stat1",
]);
export function quoteIdentifier(value: string): string {
  if (!IDENTIFIER.test(value))
    throw new Error(`Unsafe SQL identifier: ${value}`);
  return `"${value}"`;
}

export function parseWranglerRows(stdout: string): Record<string, unknown>[] {
  const parsed = JSON.parse(stdout);
  const results = Array.isArray(parsed) ? parsed : [parsed];
  if (
    results.length !== 1 ||
    results[0]?.success === false ||
    !Array.isArray(results[0]?.results)
  ) {
    throw new Error("Unexpected Wrangler D1 JSON response.");
  }
  return results[0].results;
}

export function orderTables(
  tableNames: string[],
  foreignKeys: { table: string; references: string }[],
): string[] {
  const tables = new Set(tableNames);
  const dependencies = new Map<string, Set<string>>(
    tableNames.map((name) => [name, new Set<string>()]),
  );
  for (const key of foreignKeys) {
    if (tables.has(key.table) && tables.has(key.references))
      dependencies.get(key.table)!.add(key.references);
  }
  const ordered: string[] = [];
  const pending = new Set(tableNames);
  while (pending.size) {
    const ready = [...pending].filter((table) =>
      [...dependencies.get(table)!].every((dep) => !pending.has(dep)),
    );
    if (!ready.length)
      throw new Error(
        `Foreign-key cycle in legacy tables: ${[...pending].sort().join(", ")}`,
      );
    for (const table of ready.sort()) {
      pending.delete(table);
      ordered.push(table);
    }
  }
  return ordered;
}

async function wrangler(database: string, query: string) {
  const executable = fileURLToPath(
    new URL("../node_modules/wrangler/bin/wrangler.js", import.meta.url),
  );
  const { stdout } = await new Promise<{ stdout: string }>(
    (resolvePromise, reject) => {
      const child = spawn(
        process.execPath,
        [
          executable,
          "d1",
          "execute",
          database,
          "--remote",
          "--json",
          "--command",
          query,
        ],
        { shell: false, windowsHide: true },
      );
      let stdout = "";
      let stderr = "";
      let settled = false;
      child.stdout.setEncoding("utf8").on("data", (chunk: string) => {
        stdout += chunk;
        if (stdout.length > 32 * 1024 * 1024 && !settled) {
          settled = true;
          child.kill();
          reject(
            new Error(
              "Wrangler output exceeded 32 MiB; reduce the copy page size.",
            ),
          );
        }
      });
      child.stderr
        .setEncoding("utf8")
        .on("data", (chunk: string) => (stderr += chunk));
      child.once("error", (error) => {
        if (!settled) {
          settled = true;
          reject(error);
        }
      });
      child.once("close", (code) => {
        if (settled) return;
        settled = true;
        if (code !== 0)
          reject(new Error(`Wrangler exited ${code}: ${stderr.slice(-4000)}`));
        else resolvePromise({ stdout });
      });
    },
  );
  return parseWranglerRows(stdout);
}

async function inspectSource(database: string) {
  const tables = await wrangler(
    database,
    "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name",
  );
  const names = tables
    .map((row) => row.name)
    .filter(
      (name): name is string =>
        typeof name === "string" &&
        !name.startsWith("_cf_") &&
        !INTERNAL_TABLES.has(name),
    );
  const columns = new Map<string, string[]>();
  const primaryKeys = new Map<string, string[]>();
  const foreignKeys: { table: string; references: string }[] = [];
  for (const table of names) {
    quoteIdentifier(table);
    const fields = await wrangler(
      database,
      `PRAGMA table_info(${quoteIdentifier(table)})`,
    );
    const columnNames = fields.map((field) => field.name as string);
    if (
      !columnNames.length ||
      columnNames.some(
        (name) => typeof name !== "string" || !IDENTIFIER.test(name),
      )
    ) {
      throw new Error(`D1 returned invalid columns for ${table}.`);
    }
    columns.set(table, columnNames);
    primaryKeys.set(
      table,
      fields
        .filter((field) => Number(field.pk) > 0)
        .sort((a, b) => Number(a.pk) - Number(b.pk))
        .map((field) => field.name as string),
    );
    const refs = await wrangler(
      database,
      `PRAGMA foreign_key_list(${quoteIdentifier(table)})`,
    );
    for (const ref of refs) {
      if (typeof ref.table !== "string")
        throw new Error(`Invalid foreign-key reference in ${table}.`);
      foreignKeys.push({ table, references: ref.table });
    }
  }
  return {
    names,
    columns,
    primaryKeys,
    foreignKeys,
    ordered: orderTables(names, foreignKeys),
  };
}

async function inspectTarget(
  sql: ReturnType<typeof postgres>,
  source: Awaited<ReturnType<typeof inspectSource>>,
) {
  const rows =
    await sql`SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = 'public'`;
  const columns = new Map<string, Set<string>>();
  for (const row of rows) {
    const current = columns.get(row.table_name) ?? new Set();
    current.add(row.column_name);
    columns.set(row.table_name, current);
  }
  for (const table of source.ordered) {
    const actual = columns.get(table);
    if (!actual)
      throw new Error(
        `Target table ${table} is missing. Apply the Drizzle Goose baseline first.`,
      );
    const missing = source.columns
      .get(table)!
      .filter((name) => !actual.has(name));
    if (missing.length)
      throw new Error(
        `Target table ${table} is missing columns: ${missing.join(", ")}`,
      );
  }
}

export async function main() {
  const args = new Set(process.argv.slice(2));
  const database = process.env.LEGACY_D1_DATABASE ?? "DB";
  const targetUrl = process.env.TARGET_DATABASE_URL;
  if (!targetUrl)
    throw new Error("Set TARGET_DATABASE_URL to the Postgres destination.");
  if (args.has("--help")) {
    console.log(
      "Dry run by default. Set LEGACY_D1_DATABASE and TARGET_DATABASE_URL. Pass --apply to copy rows.",
    );
    return;
  }
  const apply = args.has("--apply");
  const source = await inspectSource(database);
  const sql = postgres(targetUrl, {
    max: 1,
    connect_timeout: 10,
    idle_timeout: 5,
    prepare: false,
  });
  try {
    await inspectTarget(sql, source);
    const summary: { table: string; rows: number }[] = [];
    for (const table of source.ordered) {
      const quotedTable = quoteIdentifier(table);
      const quotedColumns = source.columns.get(table)!.map(quoteIdentifier);
      let offset = 0;
      let count = 0;
      while (true) {
        const orderBy = source.primaryKeys.get(table) ?? [];
        if (!orderBy.length)
          throw new Error(
            `No primary key is available for stable paging of ${table}.`,
          );
        if (!apply) {
          const countRows = await wrangler(
            database,
            `SELECT COUNT(*) AS count FROM ${quotedTable}`,
          );
          count = Number(countRows[0]?.count ?? 0);
          break;
        }
        const rows = await wrangler(
          database,
          `SELECT * FROM ${quotedTable} ORDER BY ${orderBy.map(quoteIdentifier).join(", ")} LIMIT ${PAGE_SIZE} OFFSET ${offset}`,
        );
        if (!rows.length) break;
        if (apply) {
          const conflict = `ON CONFLICT DO NOTHING`;
          const query = `INSERT INTO ${quotedTable} (${quotedColumns.join(", ")}) VALUES `;
          await sql.begin(async (tx) => {
            for (const row of rows) {
              const values = source.columns
                .get(table)!
                .map((column) => row[column] ?? null);
              await tx.unsafe(
                query +
                  `(${values.map((_, index) => `$${index + 1}`).join(", ")}) ${conflict}`,
                values as (string | number | boolean | null)[],
              );
            }
          });
        }
        count += rows.length;
        offset += rows.length;
        if (rows.length < PAGE_SIZE) break;
      }
      summary.push({ table, rows: count });
    }
    console.log(
      JSON.stringify(
        {
          mode: apply ? "apply" : "dry-run",
          tables: summary,
          totalRows: summary.reduce((sum, row) => sum + row.rows, 0),
        },
        null,
        2,
      ),
    );
  } finally {
    await sql.end({ timeout: 5 });
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
