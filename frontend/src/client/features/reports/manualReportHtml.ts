type ManualReportFields = {
  title: string;
  summary: string;
  body: string;
};

function escapeHtml(value: string) {
  return value.replace(/[&<>"']/g, (character) => {
    switch (character) {
      case "&":
        return "&amp;";
      case "<":
        return "&lt;";
      case ">":
        return "&gt;";
      case '"':
        return "&quot;";
      default:
        return "&#39;";
    }
  });
}

function paragraphs(value: string) {
  return value
    .trim()
    .split(/\n\s*\n/)
    .filter(Boolean)
    .map(
      (paragraph) => `<p>${escapeHtml(paragraph).replace(/\n/g, "<br>")}</p>`,
    )
    .join("\n");
}

/** Build a small, self-contained report document from plain text fields. */
export function buildManualReportHtml(fields: ManualReportFields) {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(fields.title.trim())}</title>
<style>
body{margin:0;background:#f5f7f6;color:#17201c;font:16px/1.65 system-ui,sans-serif}
main{max-width:880px;margin:0 auto;padding:48px 28px;background:#fff;min-height:100vh}
h1{margin:0 0 12px;font-size:32px;line-height:1.2}
.summary{margin:0 0 32px;padding:16px 20px;border-left:4px solid #8ac926;background:#f4f8ed}
.content p{margin:0 0 16px;white-space:pre-wrap}
@media(max-width:600px){main{padding:28px 18px}h1{font-size:26px}}
</style>
</head>
<body><main>
<h1>${escapeHtml(fields.title.trim())}</h1>
<p class="summary">${escapeHtml(fields.summary.trim())}</p>
<section class="content">${paragraphs(fields.body)}</section>
</main></body>
</html>`;
}

export function buildManualWorkLogHtml(fields: {
  title: string;
  summary: string;
  period: string;
  workCompleted: string;
  outcomes: string;
  evidenceUrls: string[];
  nextSteps: string;
}) {
  if (fields.evidenceUrls.length > 10) {
    throw new Error("A work log can include at most 10 evidence links.");
  }
  const evidence = fields.evidenceUrls
    .map((rawUrl) => {
      const url = safeEvidenceUrl(rawUrl);
      if (!url)
        throw new Error("Evidence links must be safe HTTP or HTTPS URLs.");
      return `<li><a href="${escapeHtml(url.toString())}" target="_blank" rel="noopener noreferrer">${escapeHtml(url.toString())}</a></li>`;
    })
    .join("\n");
  const evidenceList = evidence || "<li>No evidence links provided.</li>";
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(fields.title.trim())}</title>
<style>
body{margin:0;background:#f5f7f6;color:#17201c;font:16px/1.65 system-ui,sans-serif}
main{max-width:880px;margin:0 auto;padding:48px 28px;background:#fff;min-height:100vh}
h1{margin:0 0 12px;font-size:32px;line-height:1.2}h2{margin:28px 0 8px;font-size:19px}
.summary{margin:0 0 24px;padding:16px 20px;border-left:4px solid #8ac926;background:#f4f8ed}
.period{color:#4b5b52;font-size:14px}p{white-space:pre-wrap}li{overflow-wrap:anywhere;margin:6px 0}
@media(max-width:600px){main{padding:28px 18px}h1{font-size:26px}}
</style>
</head>
<body><main>
<p class="period">Work log · ${escapeHtml(fields.period.trim())}</p>
<h1>${escapeHtml(fields.title.trim())}</h1>
<p class="summary">${escapeHtml(fields.summary.trim())}</p>
<h2>Work completed</h2><p>${escapeHtml(fields.workCompleted.trim())}</p>
<h2>Results and impact</h2><p>${escapeHtml(fields.outcomes.trim() || "No outcome was recorded for this period.")}</p>
<h2>Evidence</h2><ul>${evidenceList}</ul>
<h2>Next steps</h2><p>${escapeHtml(fields.nextSteps.trim() || "No next steps listed.")}</p>
</main></body>
</html>`;
}

function safeEvidenceUrl(value: string) {
  if (value.length > 2_048) return null;
  try {
    const url = new URL(value);
    if (
      (url.protocol !== "https:" && url.protocol !== "http:") ||
      url.username ||
      url.password
    ) {
      return null;
    }
    return url;
  } catch {
    return null;
  }
}
