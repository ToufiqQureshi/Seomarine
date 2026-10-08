import { queryOptions } from "@tanstack/react-query";
import { z } from "zod";
import { apiRequest, shouldRetryApiError } from "@/client/lib/seomarineApi";
import {
  BRANDING_MAX_LOGO_CHARS,
  BRANDING_MAX_NAME_CHARS,
  type Branding,
  type BrandingInput,
} from "@/types/schemas/branding";

// The Go API's row shape. `updatedAt` is ISO-8601 with milliseconds.
const brandingSchema = z.object({
  brandName: z.string().min(1).max(BRANDING_MAX_NAME_CHARS),
  accentColor: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  logoDataUrl: z
    .string()
    .max(BRANDING_MAX_LOGO_CHARS)
    .regex(/^data:image\/(png|jpeg|webp|svg\+xml);base64,[A-Za-z0-9+/]+={0,2}$/)
    .nullable(),
  websiteUrl: z.string().nullable(),
  updatedAt: z.string().min(1),
}) satisfies z.ZodType<Branding>;

// The API answers null for an organization that uses the default branding.
const brandingOrNullSchema = brandingSchema.nullable();

const okSchema = z.object({ ok: z.literal(true) });

export function fetchBranding(): Promise<Branding | null> {
  return apiRequest("/api/v1/branding", brandingOrNullSchema);
}

// The error type is pinned to Error: `retry` takes an `unknown` error, which
// would otherwise widen the query's error type and break QueryState.
export const brandingQueryOptions = queryOptions<
  Branding | null,
  Error,
  Branding | null
>({
  queryKey: ["goBranding"] as const,
  queryFn: fetchBranding,
  retry: shouldRetryApiError,
});

export function saveBranding(input: BrandingInput): Promise<{ ok: true }> {
  return apiRequest("/api/v1/branding", okSchema, "POST", input);
}

export function resetBranding(): Promise<{ ok: true }> {
  return apiRequest("/api/v1/branding/reset", okSchema, "POST");
}
