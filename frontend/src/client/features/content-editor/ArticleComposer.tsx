import type { Editor } from "@tiptap/react";
import { useCallback, useRef, useState } from "react";
import { Input } from "@/client/components/ui/input";
import { Textarea } from "@/client/components/ui/textarea";
import type { ContentDraftInput } from "./scoring";
import { Field, SearchPreview } from "./EditorPanels";
import { useFreeSeoEditor } from "./FreeSeoEditorContext";
import { ImageFields } from "./ImageFields";
import { uploadContentImage } from "./contentEditorApi";
import {
  compressEditorImage,
  countArticleImages,
  MAX_ARTICLE_IMAGES,
} from "./image-processing";
import { ArticleEditor } from "./visual-editor/ArticleEditor";
import { validateImage } from "./visual-editor/image-input";
import { PreviewDialog } from "./visual-editor/PreviewDialog";

type ArticleComposerProps = {
  draft: ContentDraftInput;
  projectId: string;
  projectDomain: string;
  onTopicLink?: string;
  disabled: boolean;
  onImageUploadStateChange: (uploading: boolean) => void;
  onChange: (field: keyof ContentDraftInput, value: string) => void;
  previewOpen: boolean;
  onPreviewOpenChange: (open: boolean) => void;
};

export function ArticleComposer({
  draft,
  projectId,
  projectDomain,
  onTopicLink,
  disabled,
  onImageUploadStateChange,
  onChange,
  previewOpen,
  onPreviewOpenChange,
}: ArticleComposerProps) {
  return (
    <div className="space-y-5 rounded-xl border bg-card p-4 md:p-8">
      <div>
        <label
          htmlFor="content-draft-title"
          className="label-mono text-muted-foreground"
        >
          Article title · {draft.title.length}/180
        </label>
        <input
          id="content-draft-title"
          disabled={disabled}
          maxLength={180}
          value={draft.title}
          onChange={(event) => onChange("title", event.target.value)}
          placeholder="What should readers learn?"
          className="mt-1 block w-full border-0 bg-transparent p-0 font-serif text-4xl leading-tight outline-none placeholder:text-muted-foreground/60 md:text-5xl"
        />
        <p className="mt-1 text-xs text-muted-foreground">
          This is the page&apos;s H1. Use H2 and H3 for sections inside the
          article.
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <label
          htmlFor="content-draft-keyword"
          className="label-mono text-muted-foreground"
        >
          Target topic
        </label>
        <Input
          id="content-draft-keyword"
          disabled={disabled}
          maxLength={120}
          value={draft.targetKeyword}
          onChange={(event) => onChange("targetKeyword", event.target.value)}
          placeholder="e.g. technical SEO"
          className="h-9 w-64 max-w-full rounded-full"
        />
        {onTopicLink ? (
          <a
            className="text-sm font-medium text-primary underline-offset-4 hover:underline"
            href={onTopicLink}
          >
            See keyword data →
          </a>
        ) : null}
      </div>
      <details className="group border-t pt-3">
        <summary className="cursor-pointer list-none text-sm font-medium marker:hidden">
          <span className="mr-2 inline-block transition-transform group-open:rotate-90">
            ›
          </span>
          How it looks on Google
          <span className="ml-2 text-xs font-normal text-muted-foreground">
            URL and search description
          </span>
        </summary>
        <div className="mt-3 grid gap-4 md:grid-cols-2">
          <div className="space-y-4">
            <Field
              id="content-draft-slug"
              label="Page URL slug"
              hint="Short and readable"
            >
              <Input
                id="content-draft-slug"
                disabled={disabled}
                maxLength={300}
                value={draft.slug}
                onChange={(event) => onChange("slug", event.target.value)}
                placeholder="technical-seo-guide"
              />
            </Field>
            <Field
              id="content-draft-meta"
              label="Search description"
              hint={`${draft.metaDescription.length}/320`}
            >
              <Textarea
                id="content-draft-meta"
                disabled={disabled}
                maxLength={320}
                rows={3}
                value={draft.metaDescription}
                onChange={(event) =>
                  onChange("metaDescription", event.target.value)
                }
                placeholder="Summarize the page clearly for searchers."
              />
            </Field>
          </div>
          <SearchPreview draft={draft} domain={projectDomain} />
        </div>
      </details>
      <ArticleBody
        projectId={projectId}
        title={draft.title}
        content={draft.content}
        previewOpen={previewOpen}
        onPreviewOpenChange={onPreviewOpenChange}
        disabled={disabled}
        onUploadStateChange={onImageUploadStateChange}
        onChange={(content) => onChange("content", content)}
      />
    </div>
  );
}

function ArticleBody({
  projectId,
  title,
  content,
  disabled,
  onUploadStateChange,
  onChange,
  previewOpen,
  onPreviewOpenChange,
}: {
  projectId: string;
  title: string;
  previewOpen: boolean;
  onPreviewOpenChange: (open: boolean) => void;
  content: string;
  disabled: boolean;
  onUploadStateChange: (uploading: boolean) => void;
  onChange: (content: string) => void;
}) {
  const [editor, setEditor] = useState<Editor | null>(null);
  const { setEditor: setSharedEditor } = useFreeSeoEditor();
  const onEditorReady = useCallback(
    (next: Editor | null) => {
      setEditor(next);
      setSharedEditor(next);
    },
    [setSharedEditor],
  );
  const imageInputRef = useRef<HTMLInputElement>(null);
  const [imageFieldsOpen, setImageFieldsOpen] = useState(false);
  const [imageUrl, setImageUrl] = useState("");
  const [imageAlt, setImageAlt] = useState("");
  const [imageFile, setImageFile] = useState<File>();
  const [uploadingImage, setUploadingImage] = useState(false);
  const [imageError, setImageError] = useState("");

  // A pasted or dropped image opens the panel with the file already chosen,
  // so the writer only has to describe it: alt text is required to upload.
  const openImagePanel = useCallback((file?: File) => {
    setImageFieldsOpen(true);
    setImageError("");
    if (file) setImageFile(file);
  }, []);

  const insertImage = (url: string) => {
    if (!editor) return;
    if (countArticleImages(content) >= MAX_ARTICLE_IMAGES) {
      setImageError(
        `An article can include up to ${MAX_ARTICLE_IMAGES} images.`,
      );
      return;
    }
    try {
      editor.chain().focus().setImage(validateImage(url, imageAlt)).run();
      setImageError("");
      setImageUrl("");
      setImageAlt("");
      setImageFile(undefined);
      if (imageInputRef.current) imageInputRef.current.value = "";
      setImageFieldsOpen(false);
    } catch (cause) {
      setImageError(
        cause instanceof Error ? cause.message : "Could not add this image.",
      );
    }
  };

  const uploadAndInsertImage = async () => {
    if (!imageFile || !imageAlt.trim()) return;
    if (countArticleImages(content) >= MAX_ARTICLE_IMAGES) {
      setImageError(
        `An article can include up to ${MAX_ARTICLE_IMAGES} images.`,
      );
      return;
    }
    setUploadingImage(true);
    onUploadStateChange(true);
    setImageError("");
    try {
      const compressedFile = await compressEditorImage(imageFile);
      const uploaded = await uploadContentImage({
        data: { projectId, file: compressedFile },
      });
      insertImage(uploaded.url);
    } catch (cause) {
      setImageError(
        cause instanceof Error ? cause.message : "Could not upload this image.",
      );
    } finally {
      setUploadingImage(false);
      onUploadStateChange(false);
    }
  };

  return (
    <div className="space-y-2">
      <p id="article-editor-label" className="text-sm font-medium">
        Your article
      </p>
      {imageFieldsOpen ? (
        <ImageFields
          disabled={disabled}
          uploading={uploadingImage}
          imageUrl={imageUrl}
          imageAlt={imageAlt}
          imageFile={imageFile}
          imageError={imageError}
          imageInputRef={imageInputRef}
          onImageUrlChange={setImageUrl}
          onImageAltChange={setImageAlt}
          onImageFileChange={(file) => {
            setImageFile(file);
            setImageError("");
          }}
          onInsert={() => insertImage(imageUrl)}
          onUpload={() => void uploadAndInsertImage()}
          onClose={() => {
            setImageFieldsOpen(false);
            setImageError("");
          }}
        />
      ) : null}
      <ArticleEditor
        value={content}
        disabled={disabled}
        onChange={onChange}
        onAddImage={openImagePanel}
        onReady={onEditorReady}
      />
      <p className="text-xs text-muted-foreground">
        Select text to make it bold or add a link. Type / on an empty line for
        headings, lists, images and tables. Paste or drag an image to upload it.
      </p>
      <PreviewDialog
        open={previewOpen}
        onOpenChange={onPreviewOpenChange}
        title={title}
        content={content}
      />
    </div>
  );
}
