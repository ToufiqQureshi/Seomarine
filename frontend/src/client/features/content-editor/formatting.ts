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
