import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

const answersSchema = z.object({
  completedAt: z.string().nullable(),
  gscNudgeDismissedAt: z.string().nullable(),
  userCreatedAt: z.string().nullable(),
  answers: z.object({
    interestedFeatures: z.array(z.string()),
    workFor: z.string().nullable(),
    clientWebsiteCount: z.string().nullable(),
    foundVia: z.string().nullable(),
  }),
});

export async function getOnboardingAnswers() {
  return apiRequest(
    "/api/v1/onboarding/answers/get",
    answersSchema,
    "POST",
    {},
  );
}

export async function saveOnboardingAnswers({
  data,
}: {
  data: {
    interestedFeatures?: string[];
    workFor?: string;
    clientWebsiteCount?: string;
    foundVia?: string;
    completed?: boolean;
  };
}) {
  return apiRequest(
    "/api/v1/onboarding/answers/save",
    z.object({ ok: z.literal(true) }),
    "POST",
    data,
  );
}

export async function dismissGscNudge() {
  return apiRequest(
    "/api/v1/onboarding/gsc-nudge/dismiss",
    z.object({ ok: z.literal(true) }),
    "POST",
    {},
  );
}
