// @vitest-environment happy-dom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ArticleEditor } from "./ArticleEditor";

// React only flushes effects inside act() when this flag is set.
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const savedArticle = "## Saved heading\n\nThe saved article text.";

describe("ArticleEditor", () => {
  let host: HTMLDivElement;
  let root: Root;
  const onChange = vi.fn();

  const render = (value: string, disabled: boolean) =>
    act(() => {
      root.render(
        createElement(ArticleEditor, {
          value,
          disabled,
          onChange,
          onAddImage: () => {},
          onReady: () => {},
        }),
      );
    });

  beforeEach(() => {
    onChange.mockClear();
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  // The live bug: the page renders the editor empty and disabled while the
  // draft loads, then passes the saved Markdown and enables it. Enabling
  // emitted an "update", the empty editor was sent back as the draft, and
  // saving would have wiped the article.
  it("shows a draft that loads after the editor mounts, without overwriting it", () => {
    render("", true);
    render(savedArticle, false);

    expect(onChange).not.toHaveBeenCalled();
    expect(host.querySelector("h2")?.textContent).toBe("Saved heading");
    expect(host.textContent).toContain("The saved article text.");
  });

  it("reports a real edit as Markdown", () => {
    let editor: Parameters<Parameters<typeof ArticleEditor>[0]["onReady"]>[0] =
      null;
    act(() => {
      root.render(
        createElement(ArticleEditor, {
          value: savedArticle,
          disabled: false,
          onChange,
          onAddImage: () => {},
          onReady: (instance) => {
            editor = instance;
          },
        }),
      );
    });
    act(() => {
      editor?.chain().focus("end").insertContent(" More words.").run();
    });

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange.mock.calls[0]?.[0]).toMatch(
      /## Saved heading[\s\S]*More words\./,
    );
  });
});
