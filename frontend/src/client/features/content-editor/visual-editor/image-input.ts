import { safeHttpUrl } from "@/shared/safe-url";

/**
 * Checks an image the writer adds by URL or upload. The article is published
 * to the customer's website, so only http(s) images are allowed, and alt text
 * is required because it is what screen readers and image search read.
 */
export function validateImage(source: string, altText: string) {
  const src = safeHttpUrl(source.trim());
  if (!src) {
    throw new TypeError(
      "Use an image URL that starts with https:// or http://.",
    );
  }
  // One line, no Markdown brackets: the alt text is stored inside ![...](...).
  const alt = altText
    .replace(/[[\]\r\n]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!alt) throw new TypeError("Add a short description for accessibility.");
  return { src: new URL(src).href, alt };
}

/** Image files the editor accepts from paste and drag-and-drop. */
export function pickImageFile(files: ArrayLike<File> | null | undefined) {
  return Array.from(files ?? []).find(
    (file) => file.type === "image/png" || file.type === "image/jpeg",
  );
}
