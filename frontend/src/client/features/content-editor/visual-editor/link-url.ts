import { safeHttpUrl } from "@/shared/safe-url";

/**
 * Turns what a writer types into the link box into a link the article may
 * hold, or null. Writers type "example.com", so a bare domain gets https://.
 * Paths on the writer's own site ("/services") and "#section" stay as they
 * are because they are the internal links the SEO score asks for. Anything
 * else (javascript:, data:, user:pass@host) is refused, because the article
 * is published to the customer's website as-is.
 */
export function normalizeLinkUrl(input: string): string | null {
  const value = input.trim();
  if (!value || /\s/.test(value)) return null;
  if (/^\/(?!\/)/.test(value) || value.startsWith("#")) return value;
  if (/^mailto:[^@\s]+@[^@\s]+\.[^@\s]+$/i.test(value)) return value;
  const hasScheme = /^[a-z][a-z\d+.-]*:/i.test(value);
  if (hasScheme) return safeHttpUrl(value);
  if (value.startsWith("//")) return null;
  const withScheme = `https://${value}`;
  const url = safeHttpUrl(withScheme);
  return url && new URL(url).hostname.includes(".") ? url : null;
}
