import type { ContentBrief } from "./scoring";

const words = (value: string) =>
  value.match(/[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*/gu) ?? [];
const normalize = (value: string) =>
  value.toLocaleLowerCase().normalize("NFKC");
const STOP_WORDS = new Set(
  "a an and are as at be by for from in into is it of on or that the this to with your our their you we how what why when where guide best learn use using".split(
    " ",
  ),
);

function overlapsCompetitor(body: string, sources: string[]) {
  const shingles = (text: string) => {
    const tokens = words(normalize(text)).filter(
      (word) => !STOP_WORDS.has(word),
    );
    const result = new Set<string>();
    for (let index = 0; index <= tokens.length - 8; index++) {
      result.add(tokens.slice(index, index + 8).join(" "));
    }
    return result;
  };
  const draft = shingles(body);
  return sources.some((source) => {
    const sourceShingles = shingles(source);
    let matches = 0;
    for (const phrase of draft) if (sourceShingles.has(phrase)) matches++;
    return draft.size >= 4 && matches >= 4 && matches / draft.size >= 0.2;
  });
}

export function seoScoreCap(
  body: string,
  topic: string,
  brief?: ContentBrief,
  checkCopyOverlap = true,
): { cap?: number; reason?: string; blocksPublish: boolean } {
  const topicPattern = topic
    ? new RegExp(
        "(^|[^\\p{L}\\p{N}])" +
          words(normalize(topic))
            .map((word) => word.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))
            .join("[\\s\\p{P}]+") +
          "(?=$|[^\\p{L}\\p{N}])",
        "giu",
      )
    : undefined;
  const uses = topicPattern
    ? [...normalize(body).matchAll(topicPattern)].length
    : 0;
  const wordCount = words(body).length;
  const term = brief?.terms.find(
    (item) => normalize(item.term) === normalize(topic),
  );
  const normalMaximum = Math.max(term?.max ?? 0, Math.ceil(wordCount * 0.02));
  // The server refuses "Publish now" with the no-brief form of this rule
  // (backend/internal/sitepublish/stuffing.go). Change both together.
  const stuffing = Boolean(
    topic &&
    uses >= 8 &&
    uses > normalMaximum * 2 &&
    uses / Math.max(1, wordCount) > 0.04,
  );
  const caps: Array<{ cap: number; reason: string; blocksPublish: boolean }> =
    [];
  if (stuffing) {
    caps.push({
      cap: 40,
      reason: "Repeated target topic use may read as keyword stuffing.",
      blocksPublish: true,
    });
  }
  if (
    checkCopyOverlap &&
    brief &&
    overlapsCompetitor(body, brief.competitorTexts)
  ) {
    caps.push({
      cap: 30,
      reason: "Substantial phrase overlap with a competitor was found.",
      blocksPublish: false,
    });
  }
  if (wordCount < 100) {
    caps.push({
      cap: 30,
      reason: "Score is capped for drafts under 100 words.",
      blocksPublish: false,
    });
  }
  return {
    ...(caps.length
      ? {
          cap: Math.min(...caps.map((item) => item.cap)),
          reason: caps.map((item) => item.reason).join(" "),
        }
      : {}),
    blocksPublish: caps.some((item) => item.blocksPublish),
  };
}
