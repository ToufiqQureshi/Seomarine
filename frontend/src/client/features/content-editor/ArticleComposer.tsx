import { Eye, ImagePlus, Pencil } from "lucide-react";
import ReactMarkdown from "react-markdown";
import { useRef, useState } from "react";
import remarkGfm from "remark-gfm";
import { Button } from "@/client/components/ui/button";
import { Input } from "@/client/components/ui/input";
import { Textarea } from "@/client/components/ui/textarea";
import type { ContentDraftInput } from "./scoring";
import { Field } from "./EditorPanels";
import {
  formatArticleContent,
  insertImageContent,
  type ArticleFormat,
} from "./formatting";

type ArticleComposerProps = {
  draft: ContentDraftInput;
  disabled: boolean;
  onChange: (field: keyof ContentDraftInput, value: string) => void;
};

export function ArticleComposer({
  draft,
  disabled,
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
            onChange={(event) => onChange("targetKeyword", event.target.value)}
            placeholder="e.g. technical SEO"
          />
        </Field>
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
        content={draft.content}
        disabled={disabled}
        onChange={(content) => onChange("content", content)}
      />
    </div>
  );
}

function ArticleBody({
  content,
  disabled,
  onChange,
}: {
  content: string;
  disabled: boolean;
  onChange: (content: string) => void;
}) {
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const [previewMode, setPreviewMode] = useState(false);
  const [imageFieldsOpen, setImageFieldsOpen] = useState(false);
  const [imageUrl, setImageUrl] = useState("");
  const [imageAlt, setImageAlt] = useState("");
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

  const insertImage = () => {
    const editor = editorRef.current;
    const start = editor?.selectionStart ?? content.length;
    const end = editor?.selectionEnd ?? start;
    try {
      const result = insertImageContent(
        content,
        start,
        end,
        imageUrl,
        imageAlt,
      );
      onChange(result.content);
      setImageError("");
      setImageUrl("");
      setImageAlt("");
      setImageFieldsOpen(false);
      requestAnimationFrame(() => {
        editor?.focus();
        editor?.setSelectionRange(result.selectionStart, result.selectionEnd);
      });
    } catch (cause) {
      setImageError(
        cause instanceof Error ? cause.message : "Could not add this image.",
      );
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
          Heading
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
          onClick={() => formatArticle("list")}
        >
          Bullet list
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
        <div className="grid gap-3 rounded-lg border bg-muted/30 p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
          <Field
            id="content-image-url"
            label="Image URL"
            hint="Public http(s) URL"
          >
            <Input
              id="content-image-url"
              type="url"
              inputMode="url"
              autoComplete="url"
              maxLength={2_000}
              value={imageUrl}
              onChange={(event) => setImageUrl(event.target.value)}
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
              onChange={(event) => setImageAlt(event.target.value)}
              placeholder="Alt text for accessibility"
              disabled={disabled}
            />
          </Field>
          <Button
            type="button"
            size="sm"
            onClick={insertImage}
            disabled={disabled || !imageUrl.trim() || !imageAlt.trim()}
          >
            Insert image
          </Button>
          <p className="text-xs text-muted-foreground sm:col-span-3">
            Add a publicly hosted image URL. Image uploads need connected media
            storage.
          </p>
          {imageError ? (
            <p role="alert" className="text-sm text-destructive sm:col-span-3">
              {imageError}
            </p>
          ) : null}
        </div>
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
