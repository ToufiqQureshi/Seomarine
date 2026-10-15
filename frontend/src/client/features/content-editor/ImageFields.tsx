import type { RefObject } from "react";
import { Button } from "@/client/components/ui/button";
import { Input } from "@/client/components/ui/input";
import { Field } from "./EditorPanels";
import { MAX_ARTICLE_IMAGES } from "./image-processing";

export function ImageFields({
  disabled,
  uploading,
  imageUrl,
  imageAlt,
  imageFile,
  imageError,
  imageInputRef,
  onImageUrlChange,
  onImageAltChange,
  onImageFileChange,
  onInsert,
  onUpload,
  onClose,
}: {
  disabled: boolean;
  uploading: boolean;
  imageUrl: string;
  imageAlt: string;
  imageFile?: File;
  imageError: string;
  imageInputRef: RefObject<HTMLInputElement | null>;
  onImageUrlChange: (value: string) => void;
  onImageAltChange: (value: string) => void;
  onImageFileChange: (file?: File) => void;
  onInsert: () => void;
  onUpload: () => void;
  onClose: () => void;
}) {
  return (
    <div className="grid gap-3 rounded-lg border bg-muted/30 p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
      <Field id="content-image-url" label="Image URL" hint="Public http(s) URL">
        <Input
          id="content-image-url"
          type="url"
          inputMode="url"
          autoComplete="url"
          maxLength={2_000}
          value={imageUrl}
          onChange={(event) => onImageUrlChange(event.target.value)}
          placeholder="https://example.com/image.jpg"
          disabled={disabled}
        />
      </Field>
      <Field
        id="content-image-alt"
        label="Image description"
        hint="Describe what it shows"
      >
        <Input
          id="content-image-alt"
          maxLength={300}
          value={imageAlt}
          onChange={(event) => onImageAltChange(event.target.value)}
          placeholder="Alt text for accessibility"
          disabled={disabled}
        />
      </Field>
      <Button
        type="button"
        size="sm"
        onClick={onInsert}
        disabled={disabled || uploading || !imageUrl.trim() || !imageAlt.trim()}
      >
        Insert image
      </Button>
      <Field
        id="content-image-file"
        label="Or upload an image"
        hint="PNG or JPEG, up to 10 MB before compression"
      >
        <Input
          ref={imageInputRef}
          id="content-image-file"
          type="file"
          accept="image/png,image/jpeg"
          disabled={disabled || uploading}
          onChange={(event) => onImageFileChange(event.target.files?.[0])}
        />
      </Field>
      <Button
        type="button"
        size="sm"
        variant="secondary"
        onClick={onUpload}
        disabled={disabled || uploading || !imageFile || !imageAlt.trim()}
      >
        {uploading ? "Uploading…" : "Upload & insert"}
      </Button>
      {imageFile ? (
        <p className="text-sm sm:col-span-3">
          Selected: <span className="font-medium">{imageFile.name}</span>.
          Describe it above, then upload.
        </p>
      ) : null}
      <p className="text-xs text-muted-foreground sm:col-span-3">
        Uploaded images are compressed to WebP (up to 1 MB). Articles allow up
        to {MAX_ARTICLE_IMAGES} images; add alt text for accessibility.
      </p>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="justify-self-start sm:col-span-3"
        onClick={onClose}
      >
        Close
      </Button>
      {imageError ? (
        <p role="alert" className="text-sm text-destructive sm:col-span-3">
          {imageError}
        </p>
      ) : null}
    </div>
  );
}
