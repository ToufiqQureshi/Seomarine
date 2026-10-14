---
title: How to see ChatGPT traffic in Google Analytics (and why it hides)
description: Visits from ChatGPT, Perplexity and Gemini often show up as Direct. Here is how to find them in GA4 and track AI traffic properly.
date: 2026-10-15
---

**Short answer:** visits from AI assistants are often recorded as "Direct" because many of them arrive without a referrer. In Google Analytics 4 you can find part of this traffic by filtering the session source for AI domains like `chatgpt.com`, `perplexity.ai` and `gemini.google.com`, and by saving that filter as a custom channel group. To catch the rest you need analytics that also reads AI tracking tags and recognises AI assistants by name.

![The same four visits from ChatGPT, Perplexity, Gemini and Copilot shown as Direct in ordinary analytics and labelled per assistant in AI-aware analytics](/site/blog/ai-traffic-shows-as-direct.svg)

## Why AI traffic matters now

People increasingly ask an assistant before they open Google. When ChatGPT, Perplexity, Gemini or Copilot answer, they often cite a few websites and link to them. A visitor who arrives that way has usually read a summary of your page already, so they tend to land with a clear question in mind.

If those visits are hidden inside "Direct", you cannot answer simple questions:

- Is any AI assistant recommending my business at all?
- Which of my pages do they send people to?
- Is that traffic growing month on month?
- Do those visitors fill the contact form or buy?

For a small business in India, that is the difference between knowing a channel exists and guessing.

## Why it shows up as "Direct"

Analytics tools decide where a visit came from mostly by the **referrer**, the address of the page the visitor clicked on. A visit is labelled "Direct" when there is no referrer and no campaign tag.

AI visits lose the referrer in a few common ways:

- **Apps instead of browsers.** A link tapped inside the ChatGPT or Perplexity mobile app often opens without passing a referrer.
- **Privacy settings.** Some browsers and apps strip or shorten the referrer on purpose.
- **Copy and paste.** Many people copy a URL from the answer into a new tab, which carries no referrer at all.

Some assistants help by adding a tag to the link. ChatGPT, for example, adds `utm_source=chatgpt.com` to many of the links it shows, which GA4 reads as the session source. But not every assistant does this, and not on every link.

## How to find AI traffic in GA4, step by step

You can see the part of AI traffic that does carry a referrer or tag. It takes about ten minutes.

### 1. Check the sources you already have

1. Open GA4 and go to **Reports → Acquisition → Traffic acquisition**.
2. Change the primary dimension to **Session source / medium**.
3. In the search box, try each of these: `chatgpt`, `perplexity`, `gemini`, `copilot`, `claude`.

If any rows appear, AI assistants are already sending you visitors.

### 2. Build an "AI assistants" exploration

1. Go to **Explore → Blank**.
2. Add the dimensions **Session source** and **Landing page + query string**.
3. Add the metrics **Sessions**, **Engaged sessions** and **Key events**.
4. Add a filter: **Session source** *matches regex*:

```
chatgpt|perplexity|gemini\.google|copilot|claude\.ai|openai
```

Now you can see which pages AI assistants send people to, and whether those visits lead to enquiries.

### 3. Make it a permanent channel

So that AI traffic stops mixing with "Referral" in every report:

1. Go to **Admin → Data display → Channel groups → Create new channel group**.
2. Add a channel called **AI assistants** with the condition *Session source matches regex* and the pattern above.
3. **Move it above "Referral"** in the list. GA4 uses the first rule that matches.

New data will now show "AI assistants" as its own channel in your acquisition reports. GA4 applies custom channel groups to past data as well, but the visits that arrived with no referrer stay in "Direct".

## What GA4 still misses

Everything above depends on a referrer or a tag being present. Visits from apps and copied links still arrive as "Direct", and they can be a large share of AI traffic. Two signs that some of your "Direct" traffic is really AI:

- "Direct" visits landing on deep pages, such as a specific blog post or a pricing page, rather than your homepage. People rarely type those addresses by hand.
- "Direct" traffic rising on pages that you know are cited in AI answers.

Treat these as clues, not proof.

## How Uplumo tracks AI traffic

Uplumo Analytics is a small tracking script, built to answer this exact question. It:

- recognises visits from ChatGPT, Perplexity, Gemini, Claude and Copilot by their referrers **and** by AI tracking tags such as `utm_source=chatgpt.com`;
- shows AI traffic as its own channel, with the assistant and the landing page for each visit;
- sets no cookies and stores no IP addresses, so it does not need a cookie banner.

It is included in every plan, including Free. You add one script tag to your site, the same way you would add GA4, and both can run side by side.

## FAQ

**Will GA4 ever label AI traffic by itself?**
GA4's default channel groups do not have an AI channel yet, so a custom channel group is the way to do it today.

**Does traffic from Google's AI Overviews count?**
Clicks from AI Overviews arrive from Google search like any other organic click, so they appear as Organic Search, not as AI assistant traffic.

**How much AI traffic is normal?**
It varies a lot by industry and by how often assistants cite your pages. Measure your own baseline first, then watch the trend each month.

**How do I get more of it?**
Write pages that answer questions clearly, so assistants can quote them. Our guide to [answer engine optimisation](/blog/what-is-answer-engine-optimisation) covers how.
