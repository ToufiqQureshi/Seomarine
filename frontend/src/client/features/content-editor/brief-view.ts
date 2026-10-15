import type { SearchBrief } from "./contentEditorApi";
import type { ContentBrief } from "./scoring";
import { phraseCount } from "./scoring-text";

const normalizeTopic = (value: string) =>
  value.toLocaleLowerCase().replace(/\s+/g, " ").trim();

/**
 * A brief only scores the draft it was built for. When the writer changes the
 * target topic, the old brief would grade the article against the wrong
 * competitors, so it stops applying until a new one is fetched.
 */
export function briefMatchesTopic(brief: SearchBrief, topic: string) {
  return normalizeTopic(brief.keyword) === normalizeTopic(topic);
}

/** The shape the scoring engine reads (scoring.ts `ContentBrief`). */
export function toScoringBrief(brief: SearchBrief): ContentBrief {
  return {
    collectedAt: brief.fetchedAt,
    terms: brief.terms,
    subtopics: brief.subtopics,
    questions: brief.questions,
    competitorTexts: brief.competitors.map((competitor) => competitor.text),
  };
}

type TermUsage = {
  term: string;
  used: number;
  min: number;
  max: number;
  status: "under" | "in-range" | "over";
};

/** How often the draft uses each brief term, against the term's range. */
export function termUsage(brief: SearchBrief, content: string): TermUsage[] {
  return brief.terms.map((term) => {
    const used = phraseCount(content, term.term);
    const status =
      used < term.min ? "under" : used > term.max ? "over" : "in-range";
    return { term: term.term, used, min: term.min, max: term.max, status };
  });
}

const MAX_WRITER_BRIEF_CHARACTERS = 6_000;

/**
 * The part of the brief the AI writer needs: terms with ranges, questions
 * people ask, and competitor headings. Competitor body text is left out to
 * keep each action inside its 15-credit ceiling.
 */
export function briefForWriter(brief: SearchBrief | null): string {
  if (!brief) return "";
  const lines = [
    `Search brief for "${brief.keyword}" (top Google results).`,
    "Terms and how often top pages use them:",
    ...brief.terms.map((term) => `- ${term.term}: ${term.min}-${term.max}`),
    "Questions people ask:",
    ...brief.questions.slice(0, 15).map((question) => `- ${question}`),
    "Competitor headings:",
    ...brief.competitors.flatMap((competitor) =>
      competitor.headings.slice(0, 12).map((heading) => `- ${heading}`),
    ),
  ];
  const text = lines.join("\n");
  return text.length > MAX_WRITER_BRIEF_CHARACTERS
    ? text.slice(0, MAX_WRITER_BRIEF_CHARACTERS)
    : text;
}
