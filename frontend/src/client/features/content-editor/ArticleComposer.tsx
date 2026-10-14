import { Eye, ImagePlus, Pencil } from "lucide-react";
import ReactMarkdown from "react-markdown";
import { useRef, useState } from "react";
import remarkGfm from "remark-gfm";
import { Button } from "@/client/components/ui/button";
import { Input } from "@/client/components/ui/input";
import { Textarea } from "@/client/components/ui/textarea";
import type { ContentDraftInput } from "./scoring";
import { Field } from "./EditorPanels";
import { ImageFields } from "./ImageFields";
import { uploadContentImage } from "./contentEditorApi";
import {
  compressEditorImage,
  countArticleImages,
  MAX_ARTICLE_IMAGES,
} from "./image-processing";
import {
  formatArticleContent,
  insertImageContent,
  type ArticleFormat,
} from "./formatting";

type ArticleComposerProps = {
  draft: ContentDraftInput;
  projectId: string;
  onTopicLink?: string;
  disabled: boolean;
  onImageUploadStateChange: (uploading: boolean) => void;
  onChange: (field: keyof ContentDraftInput, value: string) => void;
};

export function ArticleComposer({
  draft,
  projectId,
  onTopicLink,
  disabled,
  onImageUploadStateChange,
  onChange,
}: ArticleComposerProps) {
  return (
    <div className="space-y-5 rounded-xl border bg-card p-4 md:p-6">
      <Field
        id="content-draft-title"
        label="Article title"
        hint={`${draft.title.length}/180`}
      >
        <Input
          id="content-draft-title"
          disabled={disabled}
          maxLength={180}
          value={draft.title}
          onChange={(event) => onChange("title", event.target.value)}
          placeholder="What should readers learn?"
        />
      </Field>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Field
            id="content-draft-keyword"
            label="Target topic"
            hint="Main search topic"
          >
            <Input
              id="content-draft-keyword"
              disabled={disabled}
              maxLength={120}
              value={draft.targetKeyword}
              onChange={(event) =>
                onChange("targetKeyword", event.target.value)
              }
              placeholder="e.g. technical SEO"
            />
          </Field>
          {onTopicLink ? (
            <a
              className="inline-flex text-sm font-medium text-primary underline-offset-4 hover:underline"
              href={onTopicLink}
            >
              See keyword data →
            </a>
          ) : null}
        </div>
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
      </div>
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
          onChange={(event) => onChange("metaDescription", event.target.value)}
          placeholder="Summarize the page clearly for searchers."
        />
      </Field>
      <ArticleBody
        projectId={projectId}
        content={draft.content}
        disabled={disabled}
        onUploadStateChange={onImageUploadStateChange}
        onChange={(content) => onChange("content", content)}
      />
    </div>
  );
}

function ArticleBody({
  projectId,
  content,
  disabled,
  onUploadStateChange,
  onChange,
}: {
  projectId: string;
  content: string;
  disabled: boolean;
  onUploadStateChange: (uploading: boolean) => void;
  onChange: (content: string) => void;
}) {
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const imageInputRef = useRef<HTMLInputElement>(null);
  const [previewMode, setPreviewMode] = useState(false);
  const [imageFieldsOpen, setImageFieldsOpen] = useState(false);
  const [imageUrl, setImageUrl] = useState("");
  const [imageAlt, setImageAlt] = useState("");
  const [imageFile, setImageFile] = useState<File>();
  const [uploadingImage, setUploadingImage] = useState(false);
  const [imageError, setImageError] = useState("");

  const formatArticle = (format: ArticleFormat) => {
    const editor = editorRef.current;
    const start = editor?.selectionStart ?? content.length;
    const end = editor?.selectionEnd ?? start;
    const result = formatArticleContent(content, start, end, format);
    onChange(result.content);
    requestAnimationFrame(() => {
      editor?.focus();
      editor?.setSelectionRange(result.selectionStart, result.selectionEnd);
    });
  };

  const insertImage = (url: string) => {
    if (countArticleImages(content) >= MAX_ARTICLE_IMAGES) {
      setImageError(
        `An article can include up to ${MAX_ARTICLE_IMAGES} images.`,
      );
      return false;
    }
    const editor = editorRef.current;
    const start = editor?.selectionStart ?? content.length;
    const end = editor?.selectionEnd ?? start;
    try {
      const result = insertImageContent(content, start, end, url, imageAlt);
      onChange(result.content);
      setImageError("");
      setImageUrl("");
      setImageAlt("");
      setImageFile(undefined);
      if (imageInputRef.current) imageInputRef.current.value = "";
      setImageFieldsOpen(false);
      requestAnimationFrame(() => {
        editor?.focus();
        editor?.setSelectionRange(result.selectionStart, result.selectionEnd);
      });
      return true;
    } catch (cause) {
      setImageError(
        cause instanceof Error ? cause.message : "Could not add this image.",
      );
      return false;
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
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p id="article-editor-label" className="text-sm font-medium">
          Your article
        </p>
        <span className="text-xs text-muted-foreground">
          Markdown headings, lists, links, and images
        </span>
      </div>
      <div
        className="flex flex-wrap items-center gap-1"
        role="toolbar"
        aria-label="Article tools"
      >
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("heading")}
        >
          H2
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("heading3")}
        >
          H3
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("bold")}
        >
          <strong>B</strong>
          <span className="sr-only">Bold</span>
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("italic")}
        >
          <em>I</em>
          <span className="sr-only">Italic</span>
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("list")}
        >
          Bullet list
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("numbered-list")}
        >
          Numbered list
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("quote")}
        >
          Quote
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("table")}
        >
          Table
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          onClick={() => formatArticle("link")}
        >
          Link
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled || previewMode}
          aria-expanded={imageFieldsOpen}
          onClick={() => {
            setImageFieldsOpen((open) => !open);
            setImageError("");
          }}
        >
          <ImagePlus aria-hidden />
          Image
        </Button>
        <span className="flex-1" />
        <Button
          type="button"
          variant={previewMode ? "default" : "outline"}
          size="sm"
          disabled={disabled}
          aria-pressed={previewMode}
          onClick={() => {
            setPreviewMode((preview) => !preview);
            setImageFieldsOpen(false);
          }}
        >
          {previewMode ? <Pencil aria-hidden /> : <Eye aria-hidden />}
          {previewMode ? "Write" : "Preview"}
        </Button>
      </div>
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
        />
      ) : null}
      {previewMode ? (
        <article
          aria-labelledby="article-editor-label"
          className="min-h-[440px] overflow-auto rounded-md border bg-background p-4 leading-7"
        >
          {content.trim() ? (
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
          ) : (
            <p className="text-sm text-muted-foreground">
              Your article preview will appear here.
            </p>
          )}
        </article>
      ) : (
        <Textarea
          ref={editorRef}
          id="content-draft-body"
          aria-labelledby="article-editor-label"
          disabled={disabled}
          maxLength={150_000}
          className="min-h-[440px] resize-y font-mono leading-7"
          value={content}
          onChange={(event) => onChange(event.target.value)}
          placeholder={`Start with the answer your reader needs.\n\n## Add a clear section\n\nExplain the idea in your own words...`}
        />
      )}
    </div>
  );
}
