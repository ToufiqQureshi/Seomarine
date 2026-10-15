import { Markdown } from "@tiptap/markdown";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import { BubbleMenu, FloatingMenu } from "@tiptap/react/menus";
import { useEffect, useRef, useState } from "react";
import { Textarea } from "@/client/components/ui/textarea";
import { isArticleContentWithinLimit } from "../content-limits";
import { BlockMenu, BubbleBar, LinkPanel } from "./EditorMenus";
import { EditorToolbar } from "./EditorToolbar";
import { articleExtensions } from "./extensions";
import { pickImageFile } from "./image-input";

type ArticleEditorProps = {
  value: string;
  disabled: boolean;
  onChange: (markdown: string) => void;
  /** Opens the image panel, with a file when one was pasted or dropped. */
  onAddImage: (file?: File) => void;
  /** Receives the editor so the image panel can insert into it. */
  onReady: (editor: Editor | null) => void;
};

/**
 * The article body as a visual editor. The draft stays Markdown: every change
 * is serialized with @tiptap/markdown, so scoring, saving and WordPress
 * publishing see exactly what they saw from the old Markdown field.
 */
export function ArticleEditor({
  value,
  disabled,
  onChange,
  onAddImage,
  onReady,
}: ArticleEditorProps) {
  // The last Markdown this editor produced. A different value coming in means
  // another draft was opened or the AI changed it, so the editor reloads.
  const lastMarkdown = useRef(value);
  const [markdownMode, setMarkdownMode] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  const [blockMenuOpen, setBlockMenuOpen] = useState(false);

  const editor = useEditor({
    extensions: [
      ...articleExtensions({
        placeholder:
          "Start with the answer your reader needs. Type / for headings, lists, images and tables.",
      }),
      Markdown,
    ],
    content: value,
    contentType: "markdown",
    editable: !disabled,
    shouldRerenderOnTransaction: false,
    editorProps: {
      attributes: {
        class: "article-prose min-h-[440px] px-5 py-6 outline-none md:px-10",
        "aria-labelledby": "article-editor-label",
        "aria-multiline": "true",
        role: "textbox",
      },
      handlePaste: (_view, event) => {
        const file = pickImageFile(event.clipboardData?.files);
        if (!file) return false;
        onAddImage(file);
        return true;
      },
      handleDrop: (_view, event) => {
        const file = pickImageFile(event.dataTransfer?.files);
        if (!file) return false;
        event.preventDefault();
        onAddImage(file);
        return true;
      },
      handleKeyDown: (view, event) => {
        if (event.key !== "/") return false;
        const { $from, empty } = view.state.selection;
        if (!empty || $from.parent.content.size > 0) return false;
        setBlockMenuOpen(true);
        return true;
      },
    },
    onUpdate: ({ editor: current, transaction }) => {
      // TipTap also emits "update" without a document change (for example
      // when the editor becomes editable after the draft loads). Treating
      // that as an edit sent the still-empty editor back as the draft and
      // wiped the article, so only real document changes count.
      if (!transaction.docChanged) return;
      const markdown = current.getMarkdown();
      lastMarkdown.current = markdown;
      onChange(markdown);
    },
  });

  useEffect(() => {
    onReady(editor);
    return () => onReady(null);
  }, [editor, onReady]);

  useEffect(() => {
    editor?.setEditable(!disabled, false);
  }, [editor, disabled]);

  useEffect(() => {
    if (!editor || value === lastMarkdown.current) return;
    lastMarkdown.current = value;
    editor.commands.setContent(value, {
      contentType: "markdown",
      emitUpdate: false,
    });
  }, [editor, value]);

  if (!editor) return null;
  const tooLong = !isArticleContentWithinLimit(value);

  return (
    <div className="overflow-visible rounded-lg border bg-background">
      <EditorToolbar
        editor={editor}
        disabled={disabled}
        markdownMode={markdownMode}
        onToggleMarkdown={() => setMarkdownMode((on) => !on)}
        onLink={() => setLinkOpen(true)}
        onImage={() => onAddImage()}
      />
      {linkOpen && !markdownMode ? (
        <LinkPanel editor={editor} onClose={() => setLinkOpen(false)} />
      ) : null}
      {markdownMode ? (
        <Textarea
          aria-labelledby="article-editor-label"
          className="min-h-[440px] resize-y rounded-none border-0 font-mono leading-7"
          disabled={disabled}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : (
        <>
          <EditorContent editor={editor} />
          <BubbleMenu
            editor={editor}
            className="flex items-center gap-0.5 rounded-lg bg-foreground p-1 text-background shadow-lg"
          >
            <BubbleBar editor={editor} onLink={() => setLinkOpen(true)} />
          </BubbleMenu>
          <FloatingMenu editor={editor} className="-ml-2">
            <BlockMenu
              editor={editor}
              open={blockMenuOpen}
              onOpenChange={setBlockMenuOpen}
              onImage={() => onAddImage()}
            />
          </FloatingMenu>
        </>
      )}
      {tooLong ? (
        <p role="alert" className="border-t px-4 py-2 text-sm text-destructive">
          This article exceeds the 150,000-byte limit and cannot be saved. Split
          it into two articles.
        </p>
      ) : null}
    </div>
  );
}
