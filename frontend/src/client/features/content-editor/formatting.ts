export type ArticleFormat = "heading" | "bold" | "list" | "link";

export function formatArticleContent(
  content: string,
  start: number,
  end: number,
  format: ArticleFormat,
) {
  const selected = content.slice(start, end);
  if (format === "heading" || format === "list") {
    const lineStart = content.lastIndexOf("\n", Math.max(0, start - 1)) + 1;
    const marker = format === "heading" ? "## " : "- ";
    return {
      content: `${content.slice(0, lineStart)}${marker}${content.slice(lineStart)}`,
      selectionStart: start + marker.length,
      selectionEnd: start + marker.length,
    };
  }

  const before = format === "bold" ? "**" : "[";
  const after = format === "bold" ? "**" : "](https://example.com)";
  const text = selected || (format === "link" ? "link text" : "text");
  const nextStart = start + before.length;
  return {
    content: `${content.slice(0, start)}${before}${text}${after}${content.slice(end)}`,
    selectionStart: nextStart,
    selectionEnd: nextStart + text.length,
  };
}


export function insertImageContent(
  content: string,
  start: number,
  end: number,
  source: string,
  altText: string,
) {
  const url = new URL(source.trim());
  if (url.protocol !== "https:" && url.protocol !== "http:") {
    throw new TypeError("Use an image URL that starts with https:// or http://.");
  }
  const alt = altText.trim();
  if (!alt) throw new TypeError("Add a short description for accessibility.");

  const safeAlt = alt.replace(/\\/g, "\\\\").replace(/\]/g, "\\]");
  const image = `![${safeAlt}](${url.href.replace(/\)/g, "%29")})`;
  const from = Math.max(0, Math.min(start, content.length));
  const to = Math.max(from, Math.min(end, content.length));
  const prefix = from > 0 && content[from - 1] !== "\n" ? "\n\n" : "";
  const suffix = to < content.length && content[to] !== "\n" ? "\n\n" : "";
  const inserted = `${prefix}${image}${suffix}`;

  return {
    content: `${content.slice(0, from)}${inserted}${content.slice(to)}`,
    selectionStart: from + inserted.length,
    selectionEnd: from + inserted.length,
  };
}
