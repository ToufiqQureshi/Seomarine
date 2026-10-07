import { describe, expect, it } from "vitest";
import {
  aiShareOfVisitors,
  aiSourceRows,
  channelRows,
  formatShare,
} from "./presentation";

describe("aiSourceRows", () => {
  it("lists every named assistant at zero when there is no AI traffic", () => {
    const rows = aiSourceRows([]);
    expect(rows.map((row) => row.label)).toEqual([
      "ChatGPT",
      "Perplexity",
      "Gemini",
      "Claude",
      "Copilot",
    ]);
    expect(rows.every((row) => row.visitors === 0 && row.share === 0)).toBe(
      true,
    );
  });

  it("sorts by visitors, keeps the named order for ties, and adds other AI only when it has visitors", () => {
    const rows = aiSourceRows([
      { source: "claude", visitors: 5 },
      { source: "other_ai", visitors: 5 },
      { source: "chatgpt", visitors: 10 },
    ]);
    expect(rows.map((row) => [row.key, row.visitors, row.share])).toEqual([
      ["chatgpt", 10, 0.5],
      ["claude", 5, 0.25],
      ["other_ai", 5, 0.25],
      ["perplexity", 0, 0],
      ["gemini", 0, 0],
      ["copilot", 0, 0],
    ]);
    expect(
      aiSourceRows([{ source: "other_ai", visitors: 0 }]).map((r) => r.key),
    ).not.toContain("other_ai");
  });
});

describe("channelRows", () => {
  it("drops empty channels and gives each a share of all visitors", () => {
    const rows = channelRows([
      { channel: "direct", visitors: 1 },
      { channel: "social", visitors: 0 },
      { channel: "ai", visitors: 3 },
    ]);
    expect(rows.map((row) => [row.label, row.share])).toEqual([
      ["AI assistants", 0.75],
      ["Direct", 0.25],
    ]);
  });
});

describe("aiShareOfVisitors", () => {
  it.each([
    { name: "no visits", channels: [], share: 0 },
    {
      name: "no AI channel",
      channels: [{ channel: "search" as const, visitors: 4 }],
      share: 0,
    },
    {
      name: "a mix",
      channels: [
        { channel: "ai" as const, visitors: 1 },
        { channel: "search" as const, visitors: 3 },
      ],
      share: 0.25,
    },
  ])("$name -> $share", ({ channels, share }) => {
    expect(aiShareOfVisitors(channels)).toBe(share);
  });
});

describe("formatShare", () => {
  it.each([
    [0, "0%"],
    [0.001, "<1%"],
    [0.005, "1%"],
    [0.426, "43%"],
    [1, "100%"],
  ])("%s -> %s", (share, text) => {
    expect(formatShare(share)).toBe(text);
  });
});
