---
title: Google Manual Action for Unnatural Links: What to Do Before Disavowing
description: Read the exact Search Console notice, document link removal attempts, use disavow only when justified, and submit a clear reconsideration request.
date: 2026-10-15
---

**Short answer:** Open the Manual Actions report and read the exact issue and scope. Identify links that violate Google’s spam policies, try to remove or qualify paid/manipulative links, and document your work. Use the disavow tool only when the situation meets Google’s criteria; then request reconsideration with a clear account of what changed.

![A practical troubleshooting flow for google manual action for unnatural links: what to do before disavowing](/site/blog/google-manual-action-unnatural-links-recovery.svg)

## Why this problem happens

Third-party tools may label many links “toxic,” but those scores are not Google manual actions. A manual action is different: it appears in Search Console and explains the issue. Disavowing random links can waste time and may discard signals from legitimate sites.

## Check these signals before changing the site

1. Verify that Search Console shows a manual action. Note whether it affects the whole site or specific sections and copy the issue description for your case notes.
2. Export links from Search Console and other sources, then review them by acquisition method and context. Focus on paid links, link schemes, hacked links, and manipulative patterns—not a vendor’s red score alone.
3. Record outreach attempts to remove or add `rel="sponsored"`/`nofollow` where appropriate. Keep the contact, URL, date, response, and result.
4. If there is no manual action, check for a security issue or ranking change separately. Do not treat a decline in a third-party link metric as a penalty.

## Fix it in a safe order

### 1. Remove or qualify policy-violating links

Contact site owners or vendors who placed paid/manipulative links. Stop ongoing schemes. Keep proof of changes and explain cases where removal was impossible.

### 2. Prepare a cautious disavow file only if justified

Follow Google’s current disavow guidance and file format. Use it for links you cannot remove when the manual-action or policy situation warrants it; do not disavow legitimate links solely because an SEO tool dislikes them.

### 3. Write a useful reconsideration request

State the violation, root cause, concrete cleanup, remaining links, and prevention steps. Be factual and specific; avoid blaming a former agency without evidence.

### 4. Wait for the review and monitor

After submission, check the report for status updates. Continue preventing manipulative link acquisition and keep records for the next review.

## How to tell the fix worked

The Manual Actions report should show the action removed or provide a response. Separately monitor search performance and link acquisition. A removed action does not guarantee rankings return to a previous position.

Keep a simple record for each affected URL: the problem, evidence, change, owner, and review date. This is useful when several teams touch the CMS, hosting, analytics, and content. If the same symptom returns, the record helps you spot the shared template or configuration instead of repeating one-off repairs.

## Mistakes that make this worse

Do not submit the same vague reconsideration request repeatedly. Do not bulk-disavow every external link or pay a vendor promising instant removal.

Before changing a live site, save the current configuration and test the change on a small set of representative URLs. If you are unsure whether an action removes content, changes access, or affects checkout, get the developer or site owner to review it first. Fixing the visible symptom while breaking a related page can cost more than the original search issue.

## FAQ

**Should I disavow toxic links if Search Console shows no manual action?**

Usually not based on a third-party score alone. Google recommends using the tool cautiously and describes specific cases for it.

## Related Uplumo guides

- [Ethical backlinks for small business](/blog/google-manual-action-unnatural-links-recovery)
- [Japanese seo hack spam urls recovery](/blog/japanese-seo-hack-spam-urls-recovery)
- [Google search console for small businesses](/blog/google-search-console-sitemap-couldnt-fetch)

## Sources

- [Google disavow links guidance](https://support.google.com/webmasters/answer/2648487)
- [Google manual actions report](https://support.google.com/webmasters/answer/9044175)
- [Google link spam policies](https://developers.google.com/search/docs/essentials/spam-policies)
- [Community manual action example](https://support.google.com/webmasters/thread/17028375/manual-action-issue)

This guide is educational, not a promise of rankings or a substitute for checking your own site, data, and platform rules. Google and other search systems decide what they crawl, index, and show.
