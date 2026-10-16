---
title: Out of Stock Products: Keep, Redirect or Remove the Page?
description: Choose an SEO-safe page action based on whether stock is temporary, the product will return, or a close replacement exists.
date: 2026-10-15
---

**Short answer:** Keep a temporarily unavailable product page live if it still helps shoppers, show accurate stock status, and offer a useful restock or alternative path. For a discontinued item, keep the page only when it remains useful; redirect to a genuinely close replacement when one exists, otherwise return 404/410. Match the page response to the product’s real status.

![A practical troubleshooting flow for out of stock or discontinued products: keep, redirect or remove the page?](/site/blog/out-of-stock-product-pages-seo.svg)

## Why this problem happens

Deleting every unavailable product can erase useful links and customer information; leaving a permanently dead page with a misleading 200 can disappoint shoppers and look empty to Google. The right choice depends on whether the item will return, whether demand remains, and whether a close substitute exists.

## Check these signals before changing the site

1. Confirm with inventory/business teams whether the item is temporarily unavailable, seasonal, delayed, or permanently discontinued. Set a review date for uncertain restocks.
2. Check the product URL’s organic clicks, backlinks, internal links, and customer support use. Search performance is one input, not the only reason to keep a product page.
3. Review the visible page and Product markup/feed for accurate availability, price, and variant state. Do not leave stale offer data.
4. List close replacements by actual customer need, not just a broad category. A substitute should fit the original intent.

## Fix it in a safe order

### 1. Temporary stock gap: keep the useful page

Keep a stable 200 page with clear out-of-stock information, expected timing only when verified, restock notification if offered, and links to similar in-stock products. Keep stock status accurate in structured data and feeds.

### 2. Seasonal item: plan the off-season

If the product reliably returns, keep the URL and explain the seasonality. Remove purchase options when unavailable and update offers when the item returns.

### 3. Discontinued with a close successor: redirect carefully

If a replacement satisfies the same need, move the old page to the successor and update internal links. Do not redirect to an unrelated homepage or generic category simply to avoid a 404.

### 4. No substitute or continuing value: return a real 404/410

Remove the product from active sitemaps and navigation, keep useful customer support information elsewhere, and let the old URL report that it is gone.

## How to tell the fix worked

Test the URL response and visible stock status, then inspect markup/feed data. Review click-through, conversions, restock signups, and support contacts. Revisit temporary products on the promised review date.

Keep a simple record for each affected URL: the problem, evidence, change, owner, and review date. This is useful when several teams touch the CMS, hosting, analytics, and content. If the same symptom returns, the record helps you spot the shared template or configuration instead of repeating one-off repairs.

## Mistakes that make this worse

Do not mark unavailable products as in stock to retain a rich result. Do not use a blanket redirect for every discontinued SKU.

Before changing a live site, save the current configuration and test the change on a small set of representative URLs. If you are unsure whether an action removes content, changes access, or affects checkout, get the developer or site owner to review it first. Fixing the visible symptom while breaking a related page can cost more than the original search issue.

## FAQ

**Should I keep sold-out products indexed?**

Often yes when the product is temporarily unavailable and the page remains genuinely helpful. For permanent removal, choose a close replacement or an honest 404/410.

## Related Uplumo guides

- [Ecommerce product pages not ranking](/blog/out-of-stock-product-pages-seo)
- [Product rich results price stars not showing](/blog/product-rich-results-price-stars-not-showing)
- [Soft 404 errors on live pages](/blog/soft-404-errors-on-live-pages)

## Sources

- [Google Product structured data](https://developers.google.com/search/docs/appearance/structured-data/product)
- [Google crawl troubleshooting and soft 404s](https://developers.google.com/search/docs/crawling-indexing/troubleshoot-crawling-errors)
- [Community out-of-stock page questions](https://support.google.com/webmasters/search?q=out+of+stock+product+pages+SEO)

This guide is educational, not a promise of rankings or a substitute for checking your own site, data, and platform rules. Google and other search systems decide what they crawl, index, and show.
