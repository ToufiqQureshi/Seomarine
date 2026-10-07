import { renderToStaticMarkup } from "react-dom/server";
import type { Branding } from "@/types/schemas/branding";

// The white-label bar stamped onto a report document: the organization's logo,
// name and website above the report, on screen and in the exported PDF. JSX so
// every stored value is escaped by React, not by hand.
//
// The document around it is model-written, so the bar carries its own inline
// styles behind `all:initial` and cannot inherit the report's CSS. It needs no
// relaxation of the report CSP: the logo is a data URL (`img-src data:`) and the
// styles are inline (`style-src 'unsafe-inline'`).

function BrandBar({ branding }: { branding: Branding }) {
  const font =
    'ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif';
  return (
    <div
      data-brand-bar=""
      style={{
        all: "initial",
        display: "flex",
        alignItems: "center",
        gap: "12px",
        padding: "12px 24px",
        borderBottom: `3px solid ${branding.accentColor}`,
        background: "#ffffff",
        color: "#111111",
        fontFamily: font,
        fontSize: "14px",
        printColorAdjust: "exact",
        WebkitPrintColorAdjust: "exact",
      }}
    >
      {branding.logoDataUrl ? (
        <img
          src={branding.logoDataUrl}
          alt=""
          style={{ height: "32px", width: "auto", maxWidth: "160px" }}
        />
      ) : null}
      <span style={{ fontFamily: font, fontWeight: 600, fontSize: "16px" }}>
        {branding.brandName}
      </span>
      {branding.websiteUrl ? (
        <a
          href={branding.websiteUrl}
          target="_blank"
          rel="noreferrer"
          style={{
            marginLeft: "auto",
            fontFamily: font,
            fontSize: "13px",
            color: branding.accentColor,
            textDecoration: "none",
          }}
        >
          {new URL(branding.websiteUrl).host}
        </a>
      ) : null}
    </div>
  );
}

/**
 * Inserts the bar right after the document's opening `<body>` tag. Without
 * parsing a document we did not write: a report with no `<body>` tag is served
 * unbranded rather than having markup spliced before its doctype, which would
 * drop it into quirks mode.
 */
export function withBrandBar(html: string, branding: Branding | null): string {
  if (!branding) return html;
  const bodyOpen = /<body\b[^>]*>/i.exec(html);
  if (!bodyOpen) return html;
  const at = bodyOpen.index + bodyOpen[0].length;
  const bar = renderToStaticMarkup(<BrandBar branding={branding} />);
  return html.slice(0, at) + bar + html.slice(at);
}
