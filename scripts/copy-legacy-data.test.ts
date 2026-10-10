import { describe, expect, it } from "vitest";
import {
  orderTables,
  parseWranglerRows,
  quoteIdentifier,
} from "./copy-legacy-data";

describe("legacy data copy planning", () => {
  it("accepts only simple SQL identifiers", () => {
    expect(quoteIdentifier("audit_pages")).toBe('"audit_pages"');
    expect(() => quoteIdentifier('users"; DROP TABLE projects;--')).toThrow(
      "Unsafe SQL identifier",
    );
  });

  it("parses one Wrangler JSON result and rejects malformed envelopes", () => {
    expect(
      parseWranglerRows('[{"success":true,"results":[{"id":"a"}]}]'),
    ).toEqual([{ id: "a" }]);
    expect(() => parseWranglerRows('{"success":false,"results":[]}')).toThrow(
      "Unexpected Wrangler",
    );
  });

  it("orders referenced tables before dependents and refuses cycles", () => {
    expect(
      orderTables(
        ["member", "user", "organization"],
        [
          { table: "member", references: "user" },
          { table: "member", references: "organization" },
        ],
      ),
    ).toEqual(["organization", "user", "member"]);
    expect(() =>
      orderTables(
        ["a", "b"],
        [
          { table: "a", references: "b" },
          { table: "b", references: "a" },
        ],
      ),
    ).toThrow("Foreign-key cycle");
  });
});
