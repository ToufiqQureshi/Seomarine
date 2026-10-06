import { z } from "zod";

// White-label branding for an organization's client-facing reports. One shape
// for the settings form and the server function, so the client's inline errors
// and the server's refusal agree.

/** ~150 KB of image once base64 is decoded: a logo, not a photo. */
export const BRANDING_MAX_LOGO_CHARS = 200_000;
export const BRANDING_MAX_NAME_CHARS = 60;
export const BRANDING_DEFAULT_ACCENT = "#2563eb";

// The report sandbox CSP allows `img-src data:` only, so the logo travels as a
// data URL. SVG is safe here: it renders through <img>, which never runs its
// scripts.
const LOGO_DATA_URL_PATTERN =
  /^data:image\/(png|jpeg|webp|svg\+xml);base64,[A-Za-z0-9+/]+={0,2}$/;

export const brandingInputSchema = z.object({
  brandName: z.string().trim().min(1).max(BRANDING_MAX_NAME_CHARS),
  accentColor: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  logoDataUrl: z
    .string()
    .max(BRANDING_MAX_LOGO_CHARS)
    .regex(LOGO_DATA_URL_PATTERN)
    .nullable(),
  websiteUrl: z
    .url({ protocol: /^https?$/ })
    .max(200)
    .nullable(),
});

export type BrandingInput = z.infer<typeof brandingInputSchema>;

export type Branding = BrandingInput & { updatedAt: string };
