# branding

White-label report branding for an organization. Owner: Buffy. Reserved goose
migration range: none needed (no new tables).

## Status

| Piece | State | Notes |
| --- | --- | --- |
| `backend/internal/branding` | done | Plain SQL over the unchanged `organization_branding` table. |
| `backend/api/branding.yaml` | done | GET/POST `/branding`, POST `/branding/reset`. |
| `src/client/features/branding/brandingApi.ts` | done | Calls the Go API through `apiRequest`. |
| `src/serverFunctions/branding.ts` | deleted | Replaced by the client API module. |
| `src/server/features/branding/services/BrandingService.ts` | **blocked** | Still imported by `src/routes/r/$reportId.ts`, `src/routes/s/$token/raw.ts` and `reports/sharePage.tsx`. |
| `src/server/features/branding/repositories/BrandingRepository.ts` | **blocked** | Only used through `BrandingService`. |
| `src/server/features/branding/brandBar.tsx` | **blocked** | Renders the brand bar inside the reports HTML; moves with `reports`. |

The odd-looking dates in the table are deliberate: `updatedAt` keeps the legacy
`Date#toISOString()` shape (`2006-01-02T15:04:05.000Z`) so a reader of the old
and new rows cannot tell them apart.

## Why the row is not DONE yet

`BrandingService.getBranding` is called by the legacy report routes
(`src/routes/r/$reportId.ts`, `src/routes/s/$token/raw.ts`) and by
`reports/sharePage.tsx`, all of which are still TypeScript. Deleting the
branding service now would break `tsc`/knip for the app the Go server still
proxies to. The service and repository move to Go in the `reports` PR, which is
the same change that removes those callers; `branding` flips to DONE there.

## Parity

Ported from `src/types/schemas/branding.ts` (`brandingInputSchema`) and
`brandingInputSchema`'s use in `src/serverFunctions/branding.ts`:

- `brandName` trimmed, 1..60 characters counted as UTF-16 code units, matching
  JavaScript `String#length` that zod's `max()` used. 31 astral emoji (62 units)
  are refused.
- `accentColor` must match `^#[0-9a-fA-F]{6}$`.
- `logoDataUrl` nullable, max 200 000 characters, must be a base64
  png/jpeg/webp/svg data URL.
- `websiteUrl` nullable, max 200 characters, absolute http(s) URL.
- Every key is required (zod `.nullable()` is not `.optional()`), so a missing
  key is a 400 while an explicit `null` is accepted.
- Edits need the owner or admin role, the better-auth `organization: ["update"]`
  permission; reads are open to any member.
- No row returns `null`, not an empty object.

Tests: `backend/internal/branding/branding_test.go`. The validator, role and
handler tests run with no database. The round-trip and org-isolation tests need
`TEST_DATABASE_URL` and are skipped locally, as the rest of the Go suite does.

## Known differences

- The legacy server function returned the parsed payload; the Go route answers
  `{"ok":true}`. No client read the return value.
- `PUT`/`DELETE` were considered and dropped: `apiRequest` in
  `src/client/lib/seomarineApi.ts` speaks GET and POST only, and the legacy
  functions were POSTs, so no shared client change was needed.
