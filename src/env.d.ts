// Build-time variables Vite exposes to the app (see envPrefix in vite.config.ts).
interface ImportMetaEnv {
  readonly AUTH_MODE?: "cloudflare_access" | "local_noauth" | "hosted";
  readonly BYPASS_EMAIL_VERIFICATION?: string;
  readonly POSTHOG_PUBLIC_KEY?: string;
  readonly POSTHOG_HOST?: string;
  readonly TURNSTILE_SITE_KEY?: string;
  readonly VITE_E2E_DOMAIN_FIXTURES?: string;
  readonly VITE_E2E_KEYWORD_FIXTURES?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

declare module "*.md?raw" {
  const content: string;
  export default content;
}
