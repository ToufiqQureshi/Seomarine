import { seoScoreCap } from "./scoring-caps";
import { aeo, briefSeo, staticSeo } from "./scoring-rules";
import { sumEarnedPoints as sum, type ScoreCheck } from "./scoring-utils";
export type { ScoreCheck } from "./scoring-utils";

export type EditorMode = "seo" | "aeo";
export type ContentDraftInput = {
  title: string;
  targetKeyword: string;
  slug: string;
  metaDescription: string;
  content: string;
};
export type ContentBrief = {
  collectedAt: string;
  siteDomain?: string;
  terms: Array<{ term: string; min: number; max: number; domains: number }>;
  subtopics: string[];
  questions: string[];
  competitorTexts: string[];
};
type ContentScore = {
  score: number;
  checks: ScoreCheck[];
  cappedAt?: number;
  capReason?: string;
  blocksPublish: boolean;
  hasBrief: boolean;
};
export function scoreContent(
  mode: EditorMode,
  draft: ContentDraftInput,
  brief?: ContentBrief,
): ContentScore {
  const body = draft.content.slice(0, 150000);
  let checks =
    mode === "seo"
      ? brief
        ? [
            ...briefSeo(draft, body, brief),
            ...staticSeo(draft, body, brief.siteDomain).filter((c) =>
              [
                "list-table",
                "sources-internal",
                "readability",
                "alt-text",
              ].includes(c.id),
            ),
          ]
        : staticSeo(draft, body)
      : aeo(draft, body, brief);
  let score =
      mode === "seo" && !brief
        ? Math.round(
            (sum(checks) * 100) /
              Math.max(
                1,
                checks.reduce((total, item) => total + item.weight, 0),
              ),
          )
        : sum(checks),
    scoreCap: ReturnType<typeof seoScoreCap>;
  if (
    !draft.title.trim() &&
    !draft.targetKeyword.trim() &&
    !draft.slug.trim() &&
    !draft.metaDescription.trim() &&
    !body.trim()
  ) {
    checks = checks.map((c) => ({ ...c, earned: 0, passed: false }));
    return { score: 0, checks, blocksPublish: false, hasBrief: !!brief };
  }
  scoreCap = seoScoreCap(
    body,
    draft.targetKeyword.trim(),
    brief,
    mode === "seo",
  );
  if (scoreCap.cap !== undefined) {
    score = Math.min(score, scoreCap.cap);
  }
  return {
    score,
    checks,
    ...(scoreCap.cap !== undefined
      ? { cappedAt: scoreCap.cap, capReason: scoreCap.reason }
      : {}),
    blocksPublish: scoreCap.blocksPublish,
    hasBrief: !!brief,
  };
}
