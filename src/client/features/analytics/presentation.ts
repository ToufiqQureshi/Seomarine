import type { AiSource, AnalyticsSummary, Channel } from "./analyticsApi";

const CHANNEL_LABELS: Record<Channel, { label: string; hint: string }> = {
  ai: {
    label: "AI assistants",
    hint: "ChatGPT, Perplexity, Gemini and others",
  },
  search: { label: "Search engines", hint: "Google, Bing and others" },
  social: { label: "Social media", hint: "Facebook, LinkedIn, X and others" },
  referral: { label: "Other websites", hint: "Links on other sites" },
  direct: { label: "Direct", hint: "Typed the address or used a bookmark" },
};

const AI_SOURCE_LABELS: Record<AiSource, string> = {
  chatgpt: "ChatGPT",
  perplexity: "Perplexity",
  gemini: "Gemini",
  claude: "Claude",
  copilot: "Copilot",
  other_ai: "Other AI tools",
};

// The assistants every owner asks about. Each gets a row even at zero, so
// "nobody came from Claude" reads as a fact rather than a missing row.
const NAMED_AI_SOURCES: AiSource[] = [
  "chatgpt",
  "perplexity",
  "gemini",
  "claude",
  "copilot",
];

export const DEVICE_LABELS = {
  desktop: "Desktop",
  mobile: "Mobile",
  tablet: "Tablet",
} as const;

export interface ShareRow {
  key: string;
  label: string;
  hint?: string;
  visitors: number;
  /** This row's part of the total, from 0 to 1. */
  share: number;
}

function withShares(rows: Omit<ShareRow, "share">[]): ShareRow[] {
  const total = rows.reduce((sum, row) => sum + row.visitors, 0);
  const shared = rows.map((row) => ({
    ...row,
    share: total === 0 ? 0 : row.visitors / total,
  }));
  // A fresh array, so sorting in place is safe. The sort is stable.
  shared.sort((a, b) => b.visitors - a.visitors);
  return shared;
}

/** Channels with visitors, biggest first, with their share of all visitors. */
export function channelRows(
  channels: AnalyticsSummary["channels"],
): ShareRow[] {
  return withShares(
    channels
      .filter((row) => row.visitors > 0)
      .map((row) => ({
        key: row.channel,
        ...CHANNEL_LABELS[row.channel],
        visitors: row.visitors,
      })),
  );
}

/**
 * Every named AI assistant (zero when absent) plus "Other AI tools" when it
 * has visitors, biggest first. Ties keep the named order.
 */
export function aiSourceRows(
  aiSources: AnalyticsSummary["aiSources"],
): ShareRow[] {
  const visitors = new Map(aiSources.map((row) => [row.source, row.visitors]));
  const otherAi = visitors.get("other_ai") ?? 0;
  const sources =
    otherAi > 0 ? [...NAMED_AI_SOURCES, "other_ai" as const] : NAMED_AI_SOURCES;
  return withShares(
    sources.map((source) => ({
      key: source,
      label: AI_SOURCE_LABELS[source],
      visitors: visitors.get(source) ?? 0,
    })),
  );
}

/** The AI channel's part of all visitors, from 0 to 1. */
export function aiShareOfVisitors(
  channels: AnalyticsSummary["channels"],
): number {
  return channelRows(channels).find((row) => row.key === "ai")?.share ?? 0;
}

const percent = new Intl.NumberFormat("en-IN", {
  style: "percent",
  maximumFractionDigits: 0,
});

/** "42%", or "<1%" for a share that rounds to zero but is not zero. */
export function formatShare(share: number): string {
  if (share > 0 && share < 0.005) return "<1%";
  return percent.format(share);
}
