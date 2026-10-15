import type { Editor } from "@tiptap/react";
import { useEditorState } from "@tiptap/react";
import {
  Bold,
  CircleHelp,
  Heading2,
  Heading3,
  ImagePlus,
  Italic,
  List,
  ListOrdered,
  Plus,
  Quote,
  Table,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { Button } from "@/client/components/ui/button";
import { Input } from "@/client/components/ui/input";
import { cn } from "cn";
import { normalizeLinkUrl } from "./link-url";

/** The "+" on an empty line, and the list "/" opens. */
export function BlockMenu({
  editor,
  open,
  onOpenChange,
  onImage,
}: {
  editor: Editor;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onImage: () => void;
}) {
  const run = (action: () => void) => {
    onOpenChange(false);
    action();
  };
  const items: {
    label: string;
    hint: string;
    icon: ReactNode;
    action: () => void;
  }[] = [
    {
      label: "Heading",
      hint: "Big section title",
      icon: <Heading2 aria-hidden />,
      action: () => editor.chain().focus().setHeading({ level: 2 }).run(),
    },
    {
      label: "Subheading",
      hint: "Smaller title inside a section",
      icon: <Heading3 aria-hidden />,
      action: () => editor.chain().focus().setHeading({ level: 3 }).run(),
    },
    {
      label: "Bulleted list",
      hint: "Simple list",
      icon: <List aria-hidden />,
      action: () => editor.chain().focus().toggleBulletList().run(),
    },
    {
      label: "Numbered list",
      hint: "Steps in order",
      icon: <ListOrdered aria-hidden />,
      action: () => editor.chain().focus().toggleOrderedList().run(),
    },
    {
      label: "Image",
      hint: "Upload, paste or drag",
      icon: <ImagePlus aria-hidden />,
      action: onImage,
    },
    {
      label: "Table",
      hint: "Compare options",
      icon: <Table aria-hidden />,
      action: () =>
        editor
          .chain()
          .focus()
          .insertTable({ rows: 3, cols: 2, withHeaderRow: true })
          .run(),
    },
    {
      label: "Quote",
      hint: "Highlight a line",
      icon: <Quote aria-hidden />,
      action: () => editor.chain().focus().toggleBlockquote().run(),
    },
    {
      label: "FAQ",
      hint: "Questions people ask, with answers",
      icon: <CircleHelp aria-hidden />,
      action: () =>
        editor
          .chain()
          .focus()
          .insertContent(
            "## FAQ\n\n### Question people ask?\n\nAnswer in two or three sentences.",
            { contentType: "markdown" },
          )
          .run(),
    },
  ];

  return (
    <div className="relative">
      <button
        type="button"
        aria-label="Add a block"
        aria-expanded={open}
        className="flex size-7 items-center justify-center rounded-md border bg-background text-muted-foreground hover:text-foreground"
        onClick={() => onOpenChange(!open)}
      >
        <Plus aria-hidden className="size-4" />
      </button>
      {open ? (
        <div
          role="menu"
          aria-label="Add a block"
          className="absolute top-9 left-0 z-30 w-72 rounded-xl border bg-popover p-1.5 shadow-xl"
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              onOpenChange(false);
              editor.commands.focus();
            }
          }}
        >
          {items.map((item, index) => (
            <button
              key={item.label}
              type="button"
              role="menuitem"
              // The menu opens from the keyboard ("/"), so focus lands on it.
              autoFocus={index === 0}
              className="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
              onClick={() => run(item.action)}
            >
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted [&_svg]:size-4">
                {item.icon}
              </span>
              <span className="flex flex-col leading-tight">
                <span className="text-sm font-medium">{item.label}</span>
                <span className="text-xs text-muted-foreground">
                  {item.hint}
                </span>
              </span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function LinkPanel({
  editor,
  onClose,
}: {
  editor: Editor;
  onClose: () => void;
}) {
  const currentHref: unknown = editor.getAttributes("link").href;
  const existing = typeof currentHref === "string" ? currentHref : "";
  const [input, setInput] = useState(existing);
  const [error, setError] = useState("");
  const { from, to, empty } = editor.state.selection;
  const selectedText = editor.state.doc.textBetween(from, to, " ");

  const apply = () => {
    const href = normalizeLinkUrl(input);
    if (!href) {
      setError(
        "Enter a web address like example.com, or a page on your site like /services.",
      );
      return;
    }
    if (empty && !editor.isActive("link")) {
      editor
        .chain()
        .focus()
        .insertContent({
          type: "text",
          text: input.trim(),
          marks: [{ type: "link", attrs: { href } }],
        })
        .run();
    } else {
      editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
    }
    onClose();
  };

  return (
    <form
      className="flex flex-wrap items-center gap-2 border-b bg-muted/40 px-3 py-2.5"
      onSubmit={(event) => {
        event.preventDefault();
        apply();
      }}
    >
      <label htmlFor="article-link-url" className="text-sm font-medium">
        {selectedText ? `Link for “${selectedText.slice(0, 60)}”` : "Link"}
      </label>
      <Input
        id="article-link-url"
        autoFocus
        className="min-w-56 flex-1"
        value={input}
        onChange={(event) => {
          setInput(event.target.value);
          setError("");
        }}
        placeholder="Paste a link, or a page on your site like /services"
        aria-invalid={Boolean(error)}
        aria-describedby={error ? "article-link-error" : undefined}
        onKeyDown={(event) => {
          if (event.key === "Escape") onClose();
        }}
      />
      <Button type="submit" size="sm">
        Apply
      </Button>
      {editor.isActive("link") ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            editor.chain().focus().extendMarkRange("link").unsetLink().run();
            onClose();
          }}
        >
          Remove link
        </Button>
      ) : null}
      <Button type="button" size="sm" variant="ghost" onClick={onClose}>
        Cancel
      </Button>
      {error ? (
        <p
          id="article-link-error"
          role="alert"
          className="basis-full text-sm text-destructive"
        >
          {error}
        </p>
      ) : null}
    </form>
  );
}

/** Shown over selected text: the few things writers do to a selection. */
export function BubbleBar({
  editor,
  onLink,
}: {
  editor: Editor;
  onLink: () => void;
}) {
  const state = useEditorState({
    editor,
    selector: ({ editor: current }) => ({
      bold: current.isActive("bold"),
      italic: current.isActive("italic"),
      link: current.isActive("link"),
      h2: current.isActive("heading", { level: 2 }),
      h3: current.isActive("heading", { level: 3 }),
    }),
  });
  return (
    <>
      <BubbleButton
        label="Bold"
        active={state.bold}
        onClick={() => editor.chain().focus().toggleBold().run()}
      >
        <Bold aria-hidden className="size-4" />
      </BubbleButton>
      <BubbleButton
        label="Italic"
        active={state.italic}
        onClick={() => editor.chain().focus().toggleItalic().run()}
      >
        <Italic aria-hidden className="size-4" />
      </BubbleButton>
      <button
        type="button"
        className="h-8 rounded-md bg-primary px-2.5 text-xs font-semibold text-primary-foreground"
        onClick={onLink}
      >
        {state.link ? "Edit link" : "Add link"}
      </button>
      <BubbleButton
        label="Heading 2"
        active={state.h2}
        onClick={() => editor.chain().focus().toggleHeading({ level: 2 }).run()}
      >
        <Heading2 aria-hidden className="size-4" />
      </BubbleButton>
      <BubbleButton
        label="Heading 3"
        active={state.h3}
        onClick={() => editor.chain().focus().toggleHeading({ level: 3 }).run()}
      >
        <Heading3 aria-hidden className="size-4" />
      </BubbleButton>
    </>
  );
}

function BubbleButton({
  label,
  active,
  onClick,
  children,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={active}
      title={label}
      className={cn(
        "flex size-8 items-center justify-center rounded-md hover:bg-background/15",
        active && "bg-background/20",
      )}
      onClick={onClick}
    >
      {children}
    </button>
  );
}
