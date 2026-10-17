---
title: Core Web Vitals explained: what LCP, INP and CLS mean for your site
description: A plain guide to Google's Core Web Vitals: what LCP, INP and CLS measure, the good scores to aim for, how to check them free and the fixes that help most.
date: 2026-10-15
updated: 2026-10-17
---

**Short answer:** Core Web Vitals are three measurements Google uses to judge how a page feels to real visitors: **LCP** (how fast the main content shows), **INP** (how quickly the page reacts to a tap or click) and **CLS** (how much the page jumps around while loading). A page passes when, for at least 75% of visits, LCP is 2.5 seconds or less, INP is 200 milliseconds or less and CLS is 0.1 or less.

![Core Web Vitals thresholds: LCP good up to 2.5 seconds and poor over 4, INP good up to 200 milliseconds and poor over 500, CLS good up to 0.1 and poor over 0.25](/site/blog/core-web-vitals-thresholds.svg)

## Why they matter

Core Web Vitals are part of how Google evaluates page experience. They will not lift a weak page above a much more relevant one, but when pages are similarly relevant, a faster and more stable page has an edge. More importantly, they measure things visitors feel directly. A slow, jumpy page loses enquiries no matter where it ranks.

In India this matters even more: most visitors are on mobile, many on mid-range phones and patchy networks. A page that feels fine on your office Wi-Fi can be slow for the customers you actually want.

## The three metrics, in plain words

### LCP: Largest Contentful Paint

**What it measures:** how long it takes for the biggest thing on the screen (usually the main image or headline) to appear.

| Good | Needs work | Poor |
| --- | --- | --- |
| up to 2.5 s | 2.5 to 4 s | over 4 s |

**Common causes of a slow LCP:** a large, uncompressed hero image; a slow server or hosting plan; fonts and scripts that block the page from drawing.

### INP: Interaction to Next Paint

**What it measures:** how quickly the page responds after a visitor taps, clicks or types. It replaced First Input Delay (FID) in 2024 and looks at all interactions, not just the first.

| Good | Needs work | Poor |
| --- | --- | --- |
| up to 200 ms | 200 to 500 ms | over 500 ms |

**Common causes of a poor INP:** heavy JavaScript, too many third-party scripts (chat widgets, trackers, ad tags) and large page builders doing a lot of work on every click.

### CLS: Cumulative Layout Shift

**What it measures:** how much content moves unexpectedly while the page loads. You have felt this when you go to tap a button and an ad pushes it down.

| Good | Needs work | Poor |
| --- | --- | --- |
| up to 0.1 | 0.1 to 0.25 | over 0.25 |

**Common causes of a high CLS:** images and embeds without a set width and height, banners or pop-ups inserted at the top, and web fonts that change the size of text when they load.

## Lab data vs field data

You will see two kinds of numbers, and the difference matters:

- **Field data** comes from real Chrome users who visited your page over the last 28 days (the Chrome UX Report). **This is what Google uses.** New or low-traffic sites may not have enough visits to get any.
- **Lab data** comes from a single simulated visit by a tool such as Lighthouse. It is great for finding causes, but it is one test on one simulated device, so it can differ from what your visitors experience.

Use lab data to find what to fix, and field data to confirm that the fix worked for real visitors.

## How to check your Core Web Vitals for free

1. **PageSpeed Insights** (pagespeed.web.dev): enter a URL to see field data, when there is enough of it, and a Lighthouse lab test.
2. **Google Search Console → Core Web Vitals report:** groups your pages as Good, Needs improvement or Poor, using field data, for mobile and desktop.
3. **Uplumo Site Audit:** runs the page speed test as part of a full site audit and shows real-visitor data next to the lab results, with each issue explained in plain words. The Free plan tests your homepage on mobile and desktop; paid plans test up to ten pages, one of each page type.

## The fixes that help most

Start with these. They solve the majority of problems on typical small-business sites.

### For LCP

- Compress and resize your main image, and serve it as WebP or AVIF.
- Do not lazy-load the main image at the top of the page; lazy-load images further down instead.
- Use good hosting or a CDN, and turn on caching.
- Load only the fonts and font weights you actually use.

### For INP

- Remove plugins and third-party scripts you no longer need.
- Delay chat widgets and similar tools until the visitor scrolls or interacts.
- On WordPress, avoid heavy page-builder effects on pages that matter most.

### For CLS

- Give every image, video and embed a width and height.
- Reserve space for banners and ads instead of inserting them above content.
- Avoid pop-ups that push content down on mobile.

## Fix the slowest real user journey first

Do not start by optimizing an arbitrary score. In PageSpeed Insights, open the exact URL and note whether the result is field data from real Chrome users, lab data from one test, or both. Field data helps show what users experienced over time; lab data helps reproduce and debug a page under controlled conditions. A page may have no field data when there are too few eligible visits.

Prioritize in this order:

1. Find the page templates with the largest traffic or business impact.
2. Look for the metric that fails for real users and the device group affected.
3. Use a lab run to identify likely causes on that template.
4. Make one change, then retest the same template and review field data after it has time to collect.

Typical starting points are a large hero image or server delay for LCP, heavy third-party scripts and long main-thread work for INP, and images or banners without reserved dimensions for CLS. These are diagnostic clues, not universal fixes. Measure before and after, because removing a useful widget can hurt customers even if a lab score improves.

Google's [Core Web Vitals documentation](https://developers.google.com/search/docs/appearance/core-web-vitals) defines the good thresholds at the 75th percentile: LCP at or below 2.5 seconds, INP at or below 200 milliseconds, and CLS at or below 0.1. Check the actual field status for each metric rather than averaging them into one score.

### Keep a small change log

| Date | Page template | Change | Lab result | Field result to revisit |
| --- | --- | --- | --- | --- |
| 5 Oct | Service page | Compressed and resized hero image | LCP changed in test | Search Console after enough visits |
| 12 Oct | Product page | Reserved banner space | CLS changed in test | Search Console after enough visits |

Use our [30-day SEO plan](/blog/seo-30-day-plan-small-business) to balance speed work against content and conversion fixes. If clicks also changed, diagnose the query and page mix before assuming performance was the only cause with the [Search Console guide](/blog/search-console-impressions-clicks-down).
## FAQ

**Do Core Web Vitals affect rankings?**
They are one of many signals Google uses. Relevance and quality matter more, but between similar pages, good Core Web Vitals help, and they always help conversions.

**Why do my lab and field results disagree?**
Lab tests simulate one visit on one device; field data averages thousands of real visits over 28 days. Trust field data for the final verdict.

**My site has no field data. What now?**
That is normal for new or low-traffic sites. Fix what the lab test finds, and the field data will appear once enough Chrome users visit.

**Do I need a developer?**
Not always. Image compression, removing unused plugins and setting image sizes are often things you can do yourself in your website builder. For deeper JavaScript work, a developer helps.
