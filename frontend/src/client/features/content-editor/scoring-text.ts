export const words = (value: string) =>
  value.match(/[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*/gu) ?? [];
export const normalizeText = (value: string) =>
  value.toLocaleLowerCase().normalize("NFKC");
export const countWords = (value: string) => words(value).length;
export const headings = (value: string, maxLevel = 6) =>
  value.split("\n").flatMap((line) => {
    const match = line.trim().match(/^(#{1,6})\s+(.+)$/u);
    return match && match[1].length <= maxLevel ? [match[2]] : [];
  });
export const headingsAtLevel = (value: string, level: number) =>
  value.split("\n").flatMap((line) => {
    const match = line.trim().match(/^(#{1,6})\s+(.+)$/u);
    return match && match[1].length === level ? [match[2]] : [];
  });
export function phraseCount(text: string, phrase: string) {
  const parts = words(normalizeText(phrase)).map((part) =>
    part.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
  );
  if (!parts.length) return 0;
  const pattern = new RegExp(
    "(^|[^\\p{L}\\p{N}])" +
      parts.join("[\\s\\p{P}]+") +
      "(?=$|[^\\p{L}\\p{N}])",
    "giu",
  );
  return [...normalizeText(text).matchAll(pattern)].length;
}
export const containsPhrase = (text: string, phrase: string) =>
  phraseCount(text, phrase) > 0;
export const paragraphs = (text: string) =>
  text
    .split(/\n\s*\n/u)
    .map((part) => part.trim())
    .filter(
      (part) =>
        part &&
        !/^#{1,6}\s/u.test(part) &&
        !/^\s*(?:[-*+] |\d+[.)] )/u.test(part),
    );
