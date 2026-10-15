# First-run onboarding: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. After sign-in, the redirect hook checks onboarding state and routes incomplete users to the wizard.
2. The user enters website/project details and saves answers; optional GSC connection uses the Google integration flow.
3. Go persists answers and the independent GSC nudge-dismissal state.
4. Completion routes the user to the selected project dashboard.

## Implementation map

- `useOnboardingRedirect.ts`
- `WebsiteOnboardingStep.tsx`
- `SearchConsoleOnboardingStep.tsx`
- `onboardingApi.ts`
- `backend/internal/onboarding`

## API and service dependencies

Go package: `backend/internal/onboarding`; project creation and Google OAuth are owned by `projects` and `google`.

## Cost, security and failure boundaries

- Do not mark a step complete until its server mutation succeeds.
- Onboarding and OAuth are separate states; a skipped Google connection must not block the product.
- Repeated saves and dismissals should remain idempotent.
