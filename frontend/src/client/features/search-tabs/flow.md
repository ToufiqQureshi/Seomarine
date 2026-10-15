# Search feature navigation: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. The user selects a search tab or navigates directly to its URL.
2. `useSearchTabs` maps the route/search params to a known tab and updates navigation without starting data requests itself.
3. The selected tab strip reflects the active route; each destination feature owns its own query and access checks.

## Implementation map

- `SearchTabStrip.tsx`
- `useSearchTabs.ts`
- `useSearchTabNavigation.ts`

## API and service dependencies

UI-only navigation; data dependencies are owned by the destination features (keywords, domain, search-performance, etc.).

## Cost, security and failure boundaries

- Do not put provider calls or authorization decisions in tab state.
- Unknown/stale query params should resolve to a safe default instead of opening an invalid feature.
- Keep URL state shareable and avoid resetting feature filters unnecessarily.
