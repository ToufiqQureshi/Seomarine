import { sortBy } from "remeda";
import { makeScoreCheck as check, type ScoreCheck } from "./scoring-utils";

// freshnessRule: the "Show freshness" check passes only when the draft has
// a reviewed date less than 12 months old. The check keeps its 5-point
// weight; only the source of truth changed from a guessed date in the text
// to the draft's reviewed date.

export function freshnessCheck(
  d: ContentDraftInput,
  now: Date = new Date(),
): { passed: boolean; message: string; weight: number } {
  const weight = 5;
  if (!d.lastReviewedAt?.trim()) {
    return {
      passed: false,
      message: "Mark this draft as reviewed to show readers it is fresh.",
      weight,
    };
  }
  if (!freshWithinTwelveMonths(d.lastReviewedAt, now)) {
    return {
      passed: false,
      message:
        "The last review is more than 12 months old; mark it reviewed after checking the facts.",
      weight,
    };
  }
  return {
    passed: true,
    message: "Reviewed within the last 12 months.",
    weight,
  };
}

// freshWithinTwelveMonths compares calendar months: the review stays fresh
// through the same day of the twelfth month, and a day after expires.
export function freshWithinTwelveMonths(reviewed: string, now: Date): boolean {
  const parsed = new Date(`${reviewed}T00:00:00Z`);
  if (Number.isNaN(parsed.getTime())) return false;
  const ageMonths =
    (now.getUTCFullYear() - parsed.getUTCFullYear()) * 12 +
    (now.getUTCMonth() - parsed.getUTCMonth());
  if (ageMonths < 12) return true;
  if (ageMonths > 12) return false;
  // Exactly 12 months: fresh until the same calendar day passes.
  return parsed.getUTCDate() >= now.getUTCDate();
}
import {
  containsPhrase as has,
  countWords as count,
  headings as heads,
  headingsAtLevel,
  normalizeText as norm,
  paragraphs as paras,
  phraseCount,
  words,
} from "./scoring-text";
import type { ContentBrief, ContentDraftInput } from "./scoring";

export function staticSeo(
  d: ContentDraftInput,
  body: string,
  siteDomain?: string,
): ScoreCheck[] {
  const topic = d.targetKeyword.trim(),
    h2 = headingsAtLevel(body, 2),
    places = [
      d.title,
      headingsAtLevel(body, 1)[0] ?? "",
      words(body).slice(0, 100).join(" "),
      h2.join(" "),
    ],
    placed = places.filter((p) => topic && has(p, topic)).length;
  const links = [...body.matchAll(/\[[^\]]+\]\(([^)]+)\)/giu)].map(
      (m) => m[1] ?? "",
    ),
    inside = links.filter((url) => isInternalLink(url, siteDomain)).length,
    outside = links.length - inside;
  const images = [...body.matchAll(/!\[([^\]]*)\]\(([^)]+)\)/gu)],
    alt = images.filter((m) => (m[1] ?? "").trim().length >= 8).length;
  const allWords = words(body);
  const hindiWords = new Set(
    "hai hain ka ki ke se mein me aur ko par yeh ye kya kaise nahi nahin hum aap iska uska hota hoti hote karne karega chahiye".split(
      " ",
    ),
  );
  const hindiShare =
    allWords.filter((word) => hindiWords.has(norm(word))).length /
    Math.max(1, allWords.length);
  const english =
    allWords.length > 0 && hindiShare < 0.05 && !/[\u0900-\u097f]/u.test(body);
  const pc = paras(body).map(count).filter(Boolean),
    avg = pc.length ? pc.reduce((a, b) => a + b, 0) / pc.length : 0,
    list = /^\s*(?:[-*+] |\d+[.)] )|\|.+\|/mu.test(body);
  return [
    check(
      "topic-placement",
      "Topic placement",
      topic
        ? placed + "/4: title, H1, opening 100 words and H2."
        : "Add a target topic to check placement.",
      15,
      topic ? (15 * placed) / 4 : 0,
    ),
    check(
      "headings",
      "Clear sections",
      heads(body).length + " headings found.",
      3,
      heads(body).length ? 3 : 0,
    ),
    check(
      "list-table",
      "Scannable format",
      "Use a list or table where useful.",
      5,
      list ? 5 : 0,
    ),
    check(
      "sources-internal",
      "Sources and internal links",
      outside + " external source(s), " + inside + " internal link(s).",
      8,
      Math.min(8, (outside ? 5 : 0) + (inside ? 3 : 0)),
    ),
    check(
      "readability",
      "Readable paragraphs",
      english
        ? "English paragraphs average " + Math.round(avg) + " words."
        : "No English readability penalty applied for this language.",
      7,
      allWords.length > 0 && (!english || (avg > 0 && avg <= 120)) ? 7 : 0,
    ),
    check(
      "alt-text",
      "Descriptive image alt text",
      images.length
        ? alt + "/" + images.length + " images have descriptive alt text."
        : "Not applicable: no images in this draft.",
      images.length ? 5 : 0,
      images.length ? (5 * alt) / images.length : 0,
    ),
  ];
}
function isInternalLink(url: string, siteDomain?: string) {
  if (url.startsWith("/") || url.startsWith("#")) return true;
  if (!siteDomain) return false;
  try {
    const linkedHost = new URL(url).hostname
      .toLocaleLowerCase()
      .replace(/^www\./u, "");
    const ownHost = siteDomain.toLocaleLowerCase().replace(/^www\./u, "");
    return linkedHost === ownHost;
  } catch {
    return false;
  }
}
export function briefSeo(
  d: ContentDraftInput,
  body: string,
  brief: ContentBrief,
): ScoreCheck[] {
  const terms = sortBy(
      brief.terms,
      [(term) => term.domains, "desc"],
      [(term) => term.term, "asc"],
    ).slice(0, 10),
    denom = terms.reduce((s, t) => s + Math.max(1, t.domains), 0),
    covered = terms.reduce((s, t) => {
      const n = phraseCount(body, t.term);
      const coverage =
        t.max <= 0
          ? n
            ? 0
            : 1
          : n < t.min
            ? Math.max(0, n / Math.max(1, t.min))
            : 1;
      return s + Math.max(1, t.domains) * coverage;
    }, 0),
    over = terms.filter((t) => phraseCount(body, t.term) > t.max).length;
  const topic = d.targetKeyword.trim(),
    places = [
      d.title,
      headingsAtLevel(body, 1)[0] ?? "",
      words(body).slice(0, 100).join(" "),
      headingsAtLevel(body, 2).join(" "),
    ],
    pc = places.filter((p) => topic && has(p, topic)).length,
    h2 = headingsAtLevel(body, 2),
    sh = brief.subtopics.filter((t) => h2.some((h) => has(h, t))).length;
  const lengths = sortBy(
      brief.competitorTexts.map(count).filter((n) => n > 0),
      (value) => value,
    ),
    lo = lengths.length
      ? lengths[Math.floor((lengths.length - 1) * 0.25)]
      : 200,
    hi = lengths.length
      ? lengths[Math.ceil((lengths.length - 1) * 0.75)]
      : 1800,
    n = count(body),
    depth =
      n >= lo && n <= hi
        ? 10
        : n < lo
          ? (10 * n) / Math.max(1, lo)
          : (10 * hi) / n;
  return [
    check(
      "term-coverage",
      "Important term coverage",
      terms.length + " brief terms weighted by competitor prevalence.",
      30,
      denom ? (30 * covered) / denom : 0,
    ),
    check(
      "term-ranges",
      "Terms within suggested ranges",
      over + " of " + terms.length + " terms exceed their range.",
      10,
      terms.length ? (10 * (terms.length - over)) / terms.length : 10,
    ),
    check(
      "topic-placement",
      "Topic placement",
      pc + "/4: title, H1, opening 100 words and H2.",
      15,
      (15 * pc) / 4,
    ),
    check(
      "depth",
      "Draft depth",
      n +
        " words; competitor range " +
        lo +
        "–" +
        hi +
        " words (" +
        lengths.length +
        " samples).",
      10,
      depth,
    ),
    check(
      "subtopics",
      "Brief subtopics in H2s",
      sh + "/" + brief.subtopics.length + " subtopics covered.",
      10,
      brief.subtopics.length ? (10 * sh) / brief.subtopics.length : 10,
    ),
  ];
}
export function aeo(
  d: ContentDraftInput,
  body: string,
  brief?: ContentBrief,
): ScoreCheck[] {
  const first = paras(body)[0] ?? "",
    opening = count(first),
    topic = d.targetKeyword.trim(),
    direct = opening >= 40 && opening <= 80 && (!topic || has(first, topic)),
    qheads = heads(body).filter((h) => h.includes("?") || h.includes("？")),
    questions = brief?.questions ?? [];
  const answered = questions.filter((q) => {
    const h = qheads.find((x) => has(x, q.replace(/[?？]$/u, "")));
    if (!h) return false;
    const answer =
      body.slice(body.indexOf(h) + h.length).split(/\n#{1,6}\s/u)[0] ?? "";
    return count(answer) >= 15;
  }).length;
  const list = /^\s*(?:[-*+] |\d+[.)] )|\|.+\|/mu.test(body),
    external = [...body.matchAll(/\[[^\]]+\]\(([^)]+)\)/giu)].some(
      (m) => !isInternalLink(m[1] ?? "", brief?.siteDomain),
    ),
    esc = topic.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
    definition =
      !!topic &&
      new RegExp(
        esc + "\\s+(?:is|are|means|refers to|is defined as)\\b",
        "iu",
      ).test(body),
    faq = heads(body).some((h) =>
      /\bfaq|frequently asked questions\b/iu.test(h),
    ),
    entities = new Set(
      body
        .split(/(?<=[.!?])\s+|\n+/u)
        .map((sentence) =>
          sentence.replace(/^(\s*)[A-Z][\p{L}\p{N}&.-]*/u, (initial) =>
            initial.toLocaleLowerCase(),
          ),
        )
        .join(" ")
        .match(/\b[A-Z][\p{L}\p{N}&.-]*(?:\s+[A-Z][\p{L}\p{N}&.-]*){0,3}/gu) ??
        [],
    ).size,
    freshness = freshnessCheck(d);
  return [
    check(
      "direct-answer",
      "Direct answer",
      "Opening passage has " +
        opening +
        " words and " +
        (topic
          ? (has(first, topic) ? "names" : "does not name") + " the topic."
          : "is present."),
      20,
      direct ? 20 : 0,
    ),
    check(
      "definition",
      "Define the topic",
      "Add a plain definition of the target topic or key entity.",
      10,
      definition ? 10 : 0,
    ),
    check(
      "question-coverage",
      "Answer brief questions",
      answered +
        "/" +
        questions.length +
        " questions have a matching heading and answer.",
      25,
      questions.length
        ? (25 * answered) / questions.length
        : qheads.length
          ? 15
          : 0,
    ),
    check(
      "list-table",
      "Scannable format",
      "Use a list, steps or comparison table where useful.",
      10,
      list ? 10 : 0,
    ),
    check(
      "sources",
      "Support factual claims",
      "Link reliable sources for claims readers may verify.",
      15,
      external ? 15 : 0,
    ),
    check(
      "faq",
      "FAQ section",
      "Add an FAQ section when useful.",
      10,
      faq ? 10 : 0,
    ),
    check(
      "entities",
      "Name entities clearly",
      entities + " named entities detected.",
      5,
      entities >= 2 ? 5 : entities === 1 ? 2 : 0,
    ),
    check(
      "freshness",
      "Show freshness",
      freshness.message,
      freshness.weight,
      freshness.passed ? freshness.weight : 0,
    ),
  ];
}
