/** The server limits article Markdown by its UTF-8 byte length. */
const MAX_ARTICLE_CONTENT_BYTES = 150_000;

const encoder = new TextEncoder();

export function isArticleContentWithinLimit(content: string): boolean {
  return encoder.encode(content).byteLength <= MAX_ARTICLE_CONTENT_BYTES;
}
