import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { revalidateLogic } from "@tanstack/react-form";
import { toast } from "sonner";
import { z } from "zod";
import { SectionHeader } from "@/client/components/PageHeader";
import { QueryState } from "@/client/components/QueryState";
import { useAppForm } from "@/client/components/form/useAppForm";
import { Button } from "@/client/components/ui/button";
import { Input } from "@/client/components/ui/input";
import { Label } from "@/client/components/ui/label";
import {
  getBranding,
  resetBranding,
  saveBranding,
} from "@/serverFunctions/branding";
import {
  BRANDING_DEFAULT_ACCENT,
  BRANDING_MAX_LOGO_CHARS,
  BRANDING_MAX_NAME_CHARS,
  brandingInputSchema,
  type Branding,
} from "@/types/schemas/branding";

const brandingQueryKey = ["branding"] as const;

// The form edits the website as text, where "" means none.
const formSchema = brandingInputSchema.extend({
  websiteUrl: z.union([
    z.literal(""),
    z.url({ protocol: /^https?$/, error: "Enter a full URL, like https://…" }),
  ]),
});

export function BrandingSettings() {
  const brandingQuery = useQuery({
    queryKey: brandingQueryKey,
    queryFn: () => getBranding(),
  });

  return (
    <section className="space-y-3">
      <SectionHeader title="Report branding" />
      <p className="max-w-2xl text-sm text-muted-foreground">
        White-label the reports you send to clients. Your logo, name and website
        appear at the top of every exported PDF and shared report link, in place
        of OpenSEO.
      </p>
      <QueryState
        query={brandingQuery}
        errorFallback="We couldn't load your branding."
      >
        {(branding) => <BrandingForm branding={branding} />}
      </QueryState>
    </section>
  );
}

function BrandingForm({ branding }: { branding: Branding | null }) {
  const queryClient = useQueryClient();

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: brandingQueryKey });

  const saveMutation = useMutation({
    mutationFn: (data: Parameters<typeof saveBranding>[0]["data"]) =>
      saveBranding({ data }),
    onSuccess: async () => {
      await invalidate();
      toast.success("Branding saved");
    },
  });

  const resetMutation = useMutation({
    mutationFn: () => resetBranding(),
    onSuccess: async () => {
      await invalidate();
      toast.success("Branding removed");
    },
  });

  const form = useAppForm({
    defaultValues: {
      brandName: branding?.brandName ?? "",
      accentColor: branding?.accentColor ?? BRANDING_DEFAULT_ACCENT,
      logoDataUrl: branding?.logoDataUrl ?? null,
      websiteUrl: branding?.websiteUrl ?? "",
    },
    validationLogic: revalidateLogic(),
    validators: { onDynamic: formSchema },
    onSubmit: async ({ value }) => {
      await saveMutation.mutateAsync({
        brandName: value.brandName.trim(),
        accentColor: value.accentColor,
        logoDataUrl: value.logoDataUrl,
        websiteUrl: value.websiteUrl === "" ? null : value.websiteUrl,
      });
    },
  });

  const pickLogo = (file: File | undefined) => {
    if (!file) return;
    const reader = new FileReader();
    reader.addEventListener("load", () => {
      const result = typeof reader.result === "string" ? reader.result : "";
      if (result.length > BRANDING_MAX_LOGO_CHARS) {
        toast.error("That logo is too large. Use an image under 150 KB.");
        return;
      }
      form.setFieldValue("logoDataUrl", result);
    });
    reader.readAsDataURL(file);
  };

  return (
    <form.AppForm>
      <form.Form className="grid max-w-xl gap-4">
        <form.AppField name="brandName">
          {(field) => (
            <field.TextField
              label="Brand name"
              placeholder="Acme Digital"
              maxLength={BRANDING_MAX_NAME_CHARS}
              required
            />
          )}
        </form.AppField>

        <form.AppField name="websiteUrl">
          {(field) => (
            <field.TextField
              label="Website"
              description="Optional. Shown as a link on shared reports."
              placeholder="https://acme.agency"
              type="url"
            />
          )}
        </form.AppField>

        <form.AppField name="accentColor">
          {(field) => (
            <div className="grid gap-2">
              <Label htmlFor="branding-accent">Accent color</Label>
              <div className="flex items-center gap-3">
                <input
                  id="branding-accent"
                  type="color"
                  className="h-9 w-12 cursor-pointer rounded-md border border-border bg-transparent"
                  value={field.state.value}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
                <span className="font-mono text-sm text-muted-foreground">
                  {field.state.value}
                </span>
              </div>
            </div>
          )}
        </form.AppField>

        <form.AppField name="logoDataUrl">
          {(field) => (
            <div className="grid gap-2">
              <Label htmlFor="branding-logo">Logo</Label>
              <div className="flex items-center gap-3">
                {field.state.value ? (
                  <img
                    src={field.state.value}
                    alt="Logo preview"
                    className="h-10 w-auto max-w-40 rounded border border-border bg-white p-1"
                  />
                ) : null}
                <Input
                  id="branding-logo"
                  type="file"
                  accept="image/png,image/jpeg,image/webp,image/svg+xml"
                  className="max-w-64"
                  onChange={(event) => pickLogo(event.target.files?.[0])}
                />
                {field.state.value ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => field.handleChange(null)}
                  >
                    Remove
                  </Button>
                ) : null}
              </div>
              <p className="text-sm text-muted-foreground">
                PNG, JPG, WebP or SVG, under 150 KB.
              </p>
            </div>
          )}
        </form.AppField>

        <div className="flex items-center gap-2">
          <form.SubmitButton>Save branding</form.SubmitButton>
          {branding ? (
            <Button
              variant="ghost"
              pending={resetMutation.isPending}
              onClick={() => resetMutation.mutate()}
            >
              Remove branding
            </Button>
          ) : null}
        </div>
      </form.Form>
    </form.AppForm>
  );
}
