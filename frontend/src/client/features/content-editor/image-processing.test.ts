import { afterEach, describe, expect, it, vi } from "vitest";
import {
  compressEditorImage,
  countArticleImages,
  MAX_ARTICLE_IMAGES,
} from "./image-processing";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("article images", () => {
  it("counts Markdown images, including public URLs with escaped spaces", () => {
    const content =
      "Intro\n\n![Cover](https://cdn.example/cover.webp)\n![Chart](https://cdn.example/chart%20one.webp)";
    expect(countArticleImages(content)).toBe(2);
    expect(countArticleImages("")).toBe(0);
  });

  it("downsizes and compresses a PNG into a bounded WebP file", async () => {
    const bitmap = { width: 4000, height: 3000, close: vi.fn() };
    vi.stubGlobal("createImageBitmap", vi.fn().mockResolvedValue(bitmap));
    const canvas = {
      width: 0,
      height: 0,
      getContext: vi.fn(() => ({
        fillStyle: "",
        fillRect: vi.fn(),
        drawImage: vi.fn(),
      })),
      toBlob: vi.fn((callback: BlobCallback) => {
        callback(new Blob(["compressed"], { type: "image/webp" }));
      }),
    };
    vi.stubGlobal("document", {
      // The minimal fake implements only the canvas surface used by the helper.
      // oxlint-disable-next-line typescript-eslint(no-unsafe-type-assertion)
      createElement: vi.fn(() => canvas as unknown as HTMLCanvasElement),
    });

    const image = await compressEditorImage(
      new File(["original"], "cover.png", { type: "image/png" }),
    );
    expect(image.type).toBe("image/webp");
    expect(image.name).toBe("cover.webp");
    expect(canvas.width).toBe(2400);
    expect(canvas.height).toBe(1800);
    expect(bitmap.close).toHaveBeenCalledOnce();
    expect(canvas.toBlob).toHaveBeenCalledWith(
      expect.any(Function),
      "image/webp",
      0.82,
    );
  });

  it("rejects unsupported source formats before decoding", async () => {
    await expect(
      compressEditorImage(
        new File(["svg"], "x.svg", { type: "image/svg+xml" }),
      ),
    ).rejects.toThrow("Choose a PNG or JPEG image.");
  });

  it("defines a small per-article image cap", () => {
    expect(MAX_ARTICLE_IMAGES).toBe(5);
  });
});
