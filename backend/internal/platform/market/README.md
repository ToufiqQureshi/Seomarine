# Market tables and resolution

## Source behavior and edge cases found before porting

- The shared table lists 143 countries, including Google Ads-only countries, and 128 SERP languages. Labs-only features must reject explicit Google Ads-only locations before spending provider credits.
- A project's location and language are the default pair. An explicit location override without a language uses that country's default language. An explicit language override wins.
- Labs tools replace an unsupported **project default** (country or language) with US/English. An explicitly selected unsupported country remains explicit and must be rejected by the caller.
- Labs locations have per-country language lists; the 20 multilingual overrides include India (`en`, `hi`). Google Ads-only countries have no authoritative Labs language list, so this layer defers pair validation.
- SERP rank tracking offers the full language list. Keyword-data requests fall back to the country's default when a SERP language is unsupported for that country.
- Unknown country codes retain the legacy Labs-provider and English-language fallbacks. Country ISO fallback is `us`; UK maps to ISO `gb`.
- Canonical location names can contain inconsistent comma spacing and are optionally truncated for display.

## Implementation

`generate.py` copies the legacy TypeScript source table into committed `data.json` during migration. Run it from any directory after changing `src/shared/keyword-locations.ts`; table-count tests detect incomplete regeneration. The Go package uses the generated table without network calls or mutable caches.

No database tables or environment variables. This package alone does not validate city-level SERP location codes; that belongs to the SERP location provider flow. Provider requests must validate the final Labs pair before billing.
