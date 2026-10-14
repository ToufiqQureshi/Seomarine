export const MAX_ARTICLE_IMAGES = 5;
const MAX_SOURCE_IMAGE_BYTES = 10 * 1024 * 1024;
const MAX_COMPRESSED_IMAGE_BYTES = 1024 * 1024;
const MAX_IMAGE_DIMENSION = 2400;
const MAX_IMAGE_PIXELS = 8_000_000;

export function countArticleImages(content: string) {
  return content.match(/!\[[^\]]*\]\([^\s)]+(?:\s+[^)]*)?\)/g)?.length ?? 0;
}

export async function compressEditorImage(file: File) {
  if (file.type !== "image/png" && file.type !== "image/jpeg") {
    throw new Error("Choose a PNG or JPEG image.");
  }
  if (file.size === 0 || file.size > MAX_SOURCE_IMAGE_BYTES) {
    throw new Error("Choose an image smaller than 10 MB.");
  }
  if (typeof createImageBitmap !== "function") {
    throw new Error("Image compression is not supported in this browser.");
  }

  const bitmap = await createImageBitmap(file);
  try {
    const pixelScale = Math.sqrt(
      MAX_IMAGE_PIXELS / (bitmap.width * bitmap.height),
    );
    const scale = Math.min(
      1,
      MAX_IMAGE_DIMENSION / bitmap.width,
      MAX_IMAGE_DIMENSION / bitmap.height,
      pixelScale,
    );
    const width = Math.max(1, Math.round(bitmap.width * scale));
    const height = Math.max(1, Math.round(bitmap.height * scale));
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("Could not prepare this image.");
    context.fillStyle = "#ffffff";
    context.fillRect(0, 0, width, height);
    context.drawImage(bitmap, 0, 0, width, height);

    for (const quality of [0.82, 0.72, 0.62, 0.52]) {
      const blob = await new Promise<Blob | null>((resolve) =>
        canvas.toBlob(resolve, "image/webp", quality),
      );
      if (!blob || blob.type !== "image/webp") {
        throw new Error("This browser could not compress the image to WebP.");
      }
      if (blob.size <= MAX_COMPRESSED_IMAGE_BYTES) {
        const name = file.name.replace(/\.[^.]+$/, "") || "image";
        return new File([blob], `${name}.webp`, {
          type: "image/webp",
          lastModified: Date.now(),
        });
      }
    }
    throw new Error("This image is still too large after compression.");
  } finally {
    bitmap.close();
  }
}
