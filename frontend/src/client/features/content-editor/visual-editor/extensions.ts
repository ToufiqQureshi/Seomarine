import type { AnyExtension } from "@tiptap/core";
import { Image } from "@tiptap/extension-image";
import { TableKit } from "@tiptap/extension-table";
import { Placeholder } from "@tiptap/extensions";
import { StarterKit } from "@tiptap/starter-kit";

// The draft is stored as Markdown, so the editor may only offer what Markdown
// can hold: no underline, colours or alignment.
export function articleExtensions({
  placeholder,
}: { placeholder?: string } = {}): AnyExtension[] {
  return [
    StarterKit.configure({
      underline: false,
      link: {
        openOnClick: false,
        autolink: true,
        defaultProtocol: "https",
        protocols: ["http", "https", "mailto"],
        HTMLAttributes: { rel: "noopener noreferrer", target: null },
      },
    }),
    Image.configure({ inline: false, allowBase64: false }),
    TableKit.configure({ table: { resizable: false } }),
    ...(placeholder ? [Placeholder.configure({ placeholder })] : []),
  ];
}
