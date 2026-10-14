import type { ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { MARKDOWN_COMPONENTS } from "@/client/components/Markdown";

// The shared Markdown styles are sized for chat answers. An article preview
// needs headings that read like the published page, so only those change.
const ARTICLE_COMPONENTS = {
  ...MARKDOWN_COMPONENTS,
  h1: ({ children }: { children?: ReactNode }) => (
    <h1 className="mt-6 mb-3 text-3xl font-semibold first:mt-0">{children}</h1>
  ),
  h2: ({ children }: { children?: ReactNode }) => (
    <h2 className="mt-6 mb-2 text-2xl font-semibold first:mt-0">{children}</h2>
  ),
  h3: ({ children }: { children?: ReactNode }) => (
    <h3 className="mt-5 mb-2 text-xl font-semibold first:mt-0">{children}</h3>
  ),
  img: ({ src, alt }: { src?: string | Blob; alt?: string }) =>
    typeof src === "string" ? (
      <img src={src} alt={alt ?? ""} className="my-4 rounded-md" />
    ) : null,
};

/** Renders a draft's Markdown the way a reader will see the article. */
export function ArticlePreview({ content }: { content: string }) {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={ARTICLE_COMPONENTS}>
      {content}
    </ReactMarkdown>
  );
}
