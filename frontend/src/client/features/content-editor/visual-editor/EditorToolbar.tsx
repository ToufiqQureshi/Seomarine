import type { Editor } from "@tiptap/react";
import { useEditorState } from "@tiptap/react";
import {
  Bold,
  ChevronDown,
  Code2,
  ImagePlus,
  Italic,
  Link2,
  List,
  ListOrdered,
  Redo2,
  Strikethrough,
  Table,
  Undo2,
} from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "@/client/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/client/components/ui/dropdown-menu";

const BLOCK_TYPES = [
  {
    label: "Paragraph",
    isActive: (editor: Editor) => editor.isActive("paragraph"),
  },
  {
    label: "Heading 2",
    isActive: (editor: Editor) => editor.isActive("heading", { level: 2 }),
  },
  {
    label: "Heading 3",
    isActive: (editor: Editor) => editor.isActive("heading", { level: 3 }),
  },
  {
    label: "Quote",
    isActive: (editor: Editor) => editor.isActive("blockquote"),
  },
] as const;

function setBlockType(
  editor: Editor,
  label: (typeof BLOCK_TYPES)[number]["label"],
) {
  const chain = editor.chain().focus();
  if (label === "Paragraph") chain.setParagraph().run();
  if (label === "Heading 2") chain.setHeading({ level: 2 }).run();
  if (label === "Heading 3") chain.setHeading({ level: 3 }).run();
  if (label === "Quote") chain.setParagraph().toggleBlockquote().run();
}

export function EditorToolbar({
  editor,
  disabled,
  markdownMode,
  onToggleMarkdown,
  onLink,
  onImage,
}: {
  editor: Editor;
  disabled: boolean;
  markdownMode: boolean;
  onToggleMarkdown: () => void;
  onLink: () => void;
  onImage: () => void;
}) {
  const state = useEditorState({
    editor,
    selector: ({ editor: current }) => ({
      block:
        BLOCK_TYPES.find((type) => type.isActive(current))?.label ??
        "Paragraph",
      bold: current.isActive("bold"),
      italic: current.isActive("italic"),
      strike: current.isActive("strike"),
      bulletList: current.isActive("bulletList"),
      orderedList: current.isActive("orderedList"),
      link: current.isActive("link"),
      canUndo: current.can().undo(),
      canRedo: current.can().redo(),
    }),
  });
  const off = disabled || markdownMode;

  return (
    <div
      role="toolbar"
      aria-label="Article formatting"
      className="sticky top-0 z-10 flex flex-wrap items-center gap-1 rounded-t-lg border-b bg-background px-2 py-1.5"
    >
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="outline"
              size="sm"
              disabled={off}
              className="min-w-32 justify-between"
            />
          }
        >
          {state.block}
          <ChevronDown aria-hidden />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {BLOCK_TYPES.map((type) => (
            <DropdownMenuItem
              key={type.label}
              onClick={() => setBlockType(editor, type.label)}
            >
              {type.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <Divider />
      <ToolButton
        label="Bold"
        active={state.bold}
        disabled={off}
        onClick={() => editor.chain().focus().toggleBold().run()}
      >
        <Bold aria-hidden />
      </ToolButton>
      <ToolButton
        label="Italic"
        active={state.italic}
        disabled={off}
        onClick={() => editor.chain().focus().toggleItalic().run()}
      >
        <Italic aria-hidden />
      </ToolButton>
      <ToolButton
        label="Strikethrough"
        active={state.strike}
        disabled={off}
        onClick={() => editor.chain().focus().toggleStrike().run()}
      >
        <Strikethrough aria-hidden />
      </ToolButton>
      <Divider />
      <ToolButton
        label="Bulleted list"
        active={state.bulletList}
        disabled={off}
        onClick={() => editor.chain().focus().toggleBulletList().run()}
      >
        <List aria-hidden />
      </ToolButton>
      <ToolButton
        label="Numbered list"
        active={state.orderedList}
        disabled={off}
        onClick={() => editor.chain().focus().toggleOrderedList().run()}
      >
        <ListOrdered aria-hidden />
      </ToolButton>
      <Divider />
      <Button
        type="button"
        variant={state.link ? "secondary" : "ghost"}
        size="sm"
        disabled={off}
        onClick={onLink}
      >
        <Link2 aria-hidden />
        Link
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={off}
        onClick={onImage}
      >
        <ImagePlus aria-hidden />
        Image
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={off}
        onClick={() =>
          editor
            .chain()
            .focus()
            .insertTable({ rows: 3, cols: 2, withHeaderRow: true })
            .run()
        }
      >
        <Table aria-hidden />
        Table
      </Button>
      <Divider />
      <ToolButton
        label="Undo"
        disabled={off || !state.canUndo}
        onClick={() => editor.chain().focus().undo().run()}
      >
        <Undo2 aria-hidden />
      </ToolButton>
      <ToolButton
        label="Redo"
        disabled={off || !state.canRedo}
        onClick={() => editor.chain().focus().redo().run()}
      >
        <Redo2 aria-hidden />
      </ToolButton>
      <span className="flex-1" />
      <Button
        type="button"
        variant={markdownMode ? "secondary" : "ghost"}
        size="sm"
        disabled={disabled}
        aria-pressed={markdownMode}
        onClick={onToggleMarkdown}
        title="Edit the raw Markdown"
      >
        <Code2 aria-hidden />
        Markdown
      </Button>
    </div>
  );
}

function ToolButton({
  label,
  active,
  disabled,
  onClick,
  children,
}: {
  label: string;
  active?: boolean;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Button
      type="button"
      variant={active ? "secondary" : "ghost"}
      size="icon-sm"
      aria-label={label}
      aria-pressed={active}
      title={label}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

function Divider() {
  return <span aria-hidden className="mx-1 h-5 w-px bg-border" />;
}
