import { z } from "zod";
import { apiRequest } from "@/client/lib/uplumoApi";
import { normalizeLinkUrl } from "./visual-editor/link-url";

// Free internal-link suggestions, built from the project's Search Console
// pages (or its last audit). No credits are involved.

const suggestionSchema = z.object({
  url: z.string().min(1),
  title: z.string(),
  anchor: z.string().min(1),
  reason: z.string(),
  clicks: z.number(),
  impressions: z.number(),
});

export async function fetchLinkSuggestions({
  projectId,
  content,
  targetTopic,
  title,
  slug,
}: {
  projectId: string;
  content: string;
  targetTopic: string;
  title: string;
  slug: string;
}) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/content/link-suggestions`,
    z.array(suggestionSchema),
    "POST",
    { content, targetTopic, title, slug },
  );
}

/**
 * Link the first intact anchor in the visual editor. The link command keeps
 * the change in the editor's history as one undoable transaction.
 */
type LinkChain = {
  focus: () => LinkChain;
  setTextSelection: (range: { from: number; to: number }) => LinkChain;
  setLink: (attributes: { href: string }) => LinkChain;
  run: () => boolean;
};

type LinkableEditor = {
  state: {
    doc: {
      descendants: (
        visit: (
          node: { isText: boolean; text?: string | null },
          position: number,
        ) => boolean | void,
      ) => void;
    };
  };
  chain: () => LinkChain;
};

export function applySuggestion(
  editor: LinkableEditor,
  anchor: string,
  url: string,
): boolean {
  const href = normalizeLinkUrl(url);
  if (!anchor || !href) return false;
  let range: { from: number; to: number } | null = null;
  editor.state.doc.descendants((node, position) => {
    if (range || !node.isText || !node.text) return !range;
    const index = node.text.indexOf(anchor);
    if (index < 0) return true;
    range = { from: position + index, to: position + index + anchor.length };
    return false;
  });
  if (!range) return false;
  return editor.chain().focus().setTextSelection(range).setLink({ href }).run();
}
