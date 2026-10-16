# AI Visibility and prompt research

Brand Lookup compares brand mentions and cited pages across supported answer engines; Prompt Explorer runs a prompt against selected models.

Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for the rules that protect user data, provider spend and customer trust.

## Code map

- `BrandLookupPage.tsx`
- `PromptExplorerPage.tsx`
- `aiSearchApi.ts`
- `backend/internal/aisearch/handler.go`
- `backend/internal/aisearch/service.go`
- `backend/internal/aisearch/provider.go`

- `components/BrandVisibilityHistoryCard.tsx` (saved monthly trend and separate analytics referrals)
