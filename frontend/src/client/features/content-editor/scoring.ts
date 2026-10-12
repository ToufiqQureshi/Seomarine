export type EditorMode = "seo" | "aeo";

export type ContentDraftInput = {
  title: string;
  targetKeyword: string;
  slug: string;
  metaDescription: string;
  content: string;
};

export type ScoreCheck = {
  id: string;
  label: string;
  detail: string;
  passed: boolean;
  weight: number;
};

type ContentScore = {
  score: number;
  checks: ScoreCheck[];
};

const words = (value: string) =>
  value.trim().match(/[\p{L}\p{N}]+(?:['\u2019-][\p{L}\p{N}]+)*/gu) ?? [];
const normalized = (value: string) => value.toLocaleLowerCase().trim();

function keywordMatches(value: string, keyword: string) {
  if (!keyword) return false;
  const escaped = normalized(keyword).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(
    `(^|[^\\p{L}\\p{N}])${escaped}(?=$|[^\\p{L}\\p{N}])`,
    "u",
  ).test(normalized(value));
}

function countWords(value: string) {
  return words(value.replace(/[`*_>#()[\]]/g, " ")).length;
}

function headings(content: string) {
  return content.split("\n").filter((line) => /^#{1,6}\s+\S/.test(line.trim()));
}

function checksFor(mode: EditorMode, draft: ContentDraftInput): ScoreCheck[] {
  const keyword = draft.targetKeyword.trim();
  const count = countWords(draft.content);
  const escapedKeyword = normalized(keyword).replace(
    /[.*+?^${}()|[\]\\]/g,
    "\\$&",
  );
  const keywordCount = keyword
    ? (
        normalized(draft.content).match(
          new RegExp(
            `(^|[^\\p{L}\\p{N}])${escapedKeyword}(?=$|[^\\p{L}\\p{N}])`,
            "giu",
          ),
        ) ?? []
      ).length
    : 0;
  const density = count ? (keywordCount / count) * 100 : 0;
  const bodyHeadings = headings(draft.content);
  const links = [
    ...draft.content.matchAll(/\[[^\]]+\]\((https?:\/\/[^)]+)\)/g),
  ].map((match) => match[1] ?? "");
  const firstParagraph =
    draft.content
      .split(/\n\s*\n/u)
      .map((block) => block.trim())
      .find(
        (block) => block && !block.startsWith("#") && !block.startsWith("- "),
      ) ?? "";
  const openingWords = countWords(firstParagraph);
  const paragraphs = draft.content
    .split(/\n\s*\n/u)
    .map((block) => countWords(block))
    .filter((paragraphWords) => paragraphWords > 0);
  const averageParagraph = paragraphs.length
    ? paragraphs.reduce((sum, size) => sum + size, 0) / paragraphs.length
    : 0;

  if (mode === "seo") {
    return [
      {
        id: "keyword-title",
        label: "Target keyword in title",
        detail: keyword
          ? "Use your exact target phrase in the page title."
          : "Add a target keyword first.",
        passed: keywordMatches(draft.title, keyword),
        weight: 15,
      },
      {
        id: "title-length",
        label: "Title length",
        detail: `${draft.title.length} characters; aim for 30–60 so it is less likely to be cut off.`,
        passed: draft.title.length >= 30 && draft.title.length <= 60,
        weight: 10,
      },
      {
        id: "meta-length",
        label: "Meta description length",
        detail: `${draft.metaDescription.length} characters; aim for 120–160.`,
        passed:
          draft.metaDescription.length >= 120 &&
          draft.metaDescription.length <= 160,
        weight: 10,
      },
      {
        id: "keyword-meta",
        label: "Target keyword in description",
        detail: "Mention the target phrase naturally in the search snippet.",
        passed: keywordMatches(draft.metaDescription, keyword),
        weight: 10,
      },
      {
        id: "slug",
        label: "Readable page URL",
        detail: "Use a short, readable URL with the target topic when it fits.",
        passed: Boolean(draft.slug.trim()) && !/[\sA-Z]/u.test(draft.slug),
        weight: 10,
      },
      {
        id: "keyword-coverage",
        label: "Natural topic coverage",
        detail: keyword
          ? `Keyword appears ${keywordCount} times (${density.toFixed(1)}% of words); a light, natural use is best.`
          : "Add a target keyword to measure topic coverage.",
        passed: density >= 0.3 && density <= 2.5,
        weight: 15,
      },
      {
        id: "headings",
        label: "Organized with subheadings",
        detail: `${bodyHeadings.length} Markdown heading${bodyHeadings.length === 1 ? "" : "s"} found in the draft.`,
        passed: bodyHeadings.length >= 2,
        weight: 10,
      },
      {
        id: "depth",
        label: "Useful content depth",
        detail: `${count} words; add enough detail to fully answer the reader's need (300+ is a starting signal).`,
        passed: count >= 300,
        weight: 10,
      },
      {
        id: "links",
        label: "Supporting links",
        detail: `${links.length} link${links.length === 1 ? "" : "s"} found; cite relevant sources and related pages.`,
        passed: links.length >= 1,
        weight: 10,
      },
    ];
  }

  const hasDirectAnswer = openingWords >= 20 && openingWords <= 70;
  const hasQuestions = bodyHeadings.some((heading) => heading.includes("?"));
  const hasList = draft.content
    .split("\n")
    .some((line) => /^\s*(?:[-*+] |\d+\. )/.test(line));
  const hasDefinition = /\b(?:is|are|means|refers to|defined as)\b/iu.test(
    draft.content,
  );
  const hasExternalSource = links.some((url) => !url.includes("example.com"));
  return [
    {
      id: "direct-answer",
      label: "Answer the main question early",
      detail: `Opening paragraph has ${openingWords} words; put a clear answer in the first 20–70 words.`,
      passed: hasDirectAnswer,
      weight: 20,
    },
    {
      id: "question-coverage",
      label: "Cover real reader questions",
      detail:
        "Use question-led subheadings for the questions this page answers.",
      passed: hasQuestions,
      weight: 15,
    },
    {
      id: "structured-format",
      label: "Make facts easy to extract",
      detail:
        "Use a concise list, steps, or a comparison table where it helps.",
      passed: hasList || draft.content.includes("|"),
      weight: 15,
    },
    {
      id: "definition",
      label: "Define important concepts",
      detail:
        "Clearly explain key entities, products, or terms in plain language.",
      passed: hasDefinition,
      weight: 10,
    },
    {
      id: "source",
      label: "Support claims with a source",
      detail:
        "Link to a trustworthy source for facts that readers may want to verify.",
      passed: hasExternalSource,
      weight: 15,
    },
    {
      id: "hierarchy",
      label: "Use descriptive sections",
      detail: `${bodyHeadings.length} descriptive headings; split the answer into clear sections.`,
      passed: bodyHeadings.length >= 2,
      weight: 10,
    },
    {
      id: "readability",
      label: "Keep paragraphs easy to scan",
      detail: paragraphs.length
        ? `Average paragraph length is ${Math.round(averageParagraph)} words; aim for shorter, focused blocks.`
        : "Break the draft into short, focused paragraphs.",
      passed: paragraphs.length >= 2 && averageParagraph <= 100,
      weight: 10,
    },
    {
      id: "topic-context",
      label: "Name the topic clearly",
      detail:
        "Use the target topic in the title and body so answers have enough context.",
      passed:
        keywordMatches(draft.title, keyword) &&
        keywordMatches(draft.content, keyword),
      weight: 5,
    },
  ];
}

export function scoreContent(
  mode: EditorMode,
  draft: ContentDraftInput,
): ContentScore {
  const checks = checksFor(mode, draft);
  const totalWeight = checks.reduce((total, check) => total + check.weight, 0);
  const earned = checks.reduce(
    (total, check) => total + (check.passed ? check.weight : 0),
    0,
  );
  return { score: Math.round((earned / totalWeight) * 100), checks };
}
