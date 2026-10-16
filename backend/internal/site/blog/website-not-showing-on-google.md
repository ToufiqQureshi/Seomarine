---
title: Website not showing on Google? Find and fix the cause
description: If your website or a new page is missing from Google, follow these checks for indexing, crawl blocks, page quality and sitemap mistakes.
date: 2026-10-15
---

**Short answer:** First check whether Google has indexed the exact URL in Search Console. If it has not, inspect the reason shown, confirm the page can be crawled, remove accidental blocking, make the page useful, and request a recrawl after fixing it. A sitemap helps discovery but cannot force indexing.

![A simple troubleshooting path from URL check to crawl access, page quality and indexing request](/site/blog/website-not-showing.svg)

## Check the exact URL before changing anything

Search Google for your business name and try the page’s full URL. Then open Search Console, choose URL Inspection, and test the exact address. These checks answer different questions: a search may fail to show a page that is indexed but not ranking for the query; URL Inspection tells you more about whether Google knows and can access that page.

Do not assume that every new URL should appear immediately. Google says crawling can take days or weeks, and a crawl request does not guarantee inclusion.

## Read the reason, then choose the fix

### “URL is unknown to Google”

Add an internal link to the page from a relevant page, make sure it is in the sitemap, and check that the page returns a normal success response. A sitemap is useful for discovery, especially on larger sites, but Google describes it as a hint.

### “Blocked by robots.txt” or “noindex”

Check your robots.txt file and the page’s indexing settings. A robots.txt rule controls crawling; a noindex instruction controls whether a page can appear in Search. Removing one does not automatically remove the other. Make sure the rule is intentional before changing it, especially on a live site.

### “Crawled – currently not indexed”

Google fetched the page but has not chosen to index it. Check whether it is genuinely useful and distinct. Add information that answers the searcher’s question, remove copied or repeated filler, and link to the page from related content. Publishing more pages with the same answer is not the repair.

### “Discovered – currently not indexed”

Google knows the URL but has not crawled it yet. Check whether your site creates many low-value URLs, has slow responses, or gives important pages few internal links. Clean up crawl paths and give the important page a clear route from the main navigation or a relevant guide.

### “Duplicate” or canonical mismatch

Check which URL Google selected as canonical. Similar URLs may be created by tracking parameters, filters, trailing slashes or print versions. Choose the URL that should represent the page, link to it consistently, and use the canonical tag and sitemap to support that choice.

## A safe 15-minute checklist

1. Confirm the URL loads without sign-in and returns the intended page.
2. Inspect it in Search Console and save the stated indexing reason.
3. Check robots.txt, noindex and canonical settings.
4. Check that the page has original content and a descriptive title.
5. Add one or two relevant internal links.
6. Confirm the canonical URL is the one in your sitemap.
7. Use URL Inspection to request indexing after the fix.
8. Recheck later; do not repeatedly submit the same unchanged URL.

If many URLs are missing, check the Page Indexing report and Crawl Stats report instead of testing every page one by one.

## Avoid these common mistakes

Do not create another URL just because the first one is not indexed. Do not fill the page with keywords or add fake links. Do not remove noindex directives site-wide without understanding why they were added. And do not promise a client that sitemap submission guarantees a ranking or index date.

For a fuller measurement routine, read [Google Search Console for small businesses](/blog/google-search-console-for-small-businesses), [a practical SEO audit](/blog/seo-audit-that-finds-real-problems) and [how to choose an SEO and AEO tool](/blog/choose-the-best-seo-aeo-tool).

## Check page errors and JavaScript too

A URL may be crawlable but still fail to show useful content when Google renders it. Open the page without logging in, check for server errors, and compare the visible page with what the browser receives. If your site relies on JavaScript, use URL Inspection’s rendered result where available. Make sure the main text and links are present without a visitor needing to complete a form or click a button.

Also check for a soft 404: the server may return a success response while the page says the item is gone or contains almost no useful information. If a page was removed, use a relevant replacement or a proper not-found response. Do not redirect every deleted URL to the homepage; that can confuse both visitors and crawlers.

Keep a small issue log with URL, inspection status, change and next review date. It helps a business owner and developer agree what changed and whether Google has had time to revisit it.

## FAQ

**Will a sitemap make Google index my site?**

No. It can help Google discover URLs, but it does not guarantee crawling, indexing or ranking.

**How long does indexing take?**

There is no fixed promise. Google says a recrawl may take days or weeks, and asking for one does not guarantee inclusion.

**Can a page be indexed but still not appear for my keyword?**

Yes. Indexing means Google can include the page in its index. Ranking for a particular search depends on relevance and other systems.

## Sources

- [Google: check whether Google can find and crawl your site](https://developers.google.com/search/docs/fundamentals/get-on-google)
- [Google: ask Google to recrawl a URL](https://developers.google.com/search/docs/crawling-indexing/ask-google-to-recrawl)
- [Google: technical requirements for Search](https://developers.google.com/search/docs/essentials/technical)
- [Google: sitemap overview](https://developers.google.com/search/docs/crawling-indexing/sitemaps/overview)
