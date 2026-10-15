import { Monitor, Smartphone } from "lucide-react";
import { useState } from "react";
import { Button } from "@/client/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/client/components/ui/dialog";
import { cn } from "cn";
import { ArticlePreview } from "../ArticlePreview";

/** The article as a reader will see it, on a desktop or a phone width. */
export function PreviewDialog({
  open,
  onOpenChange,
  title,
  content,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  content: string;
}) {
  const [device, setDevice] = useState<"desktop" | "mobile">("desktop");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[92vh] flex-col gap-3 p-0 sm:max-w-6xl">
        <div className="flex flex-wrap items-center gap-3 border-b px-4 py-3 pr-12">
          <DialogTitle>Preview as on website</DialogTitle>
          <DialogDescription className="sr-only">
            How the article will read once published.
          </DialogDescription>
          <div
            role="group"
            aria-label="Screen size"
            className="ml-auto flex gap-1 rounded-lg border p-0.5"
          >
            <Button
              type="button"
              size="sm"
              variant={device === "desktop" ? "secondary" : "ghost"}
              aria-pressed={device === "desktop"}
              onClick={() => setDevice("desktop")}
            >
              <Monitor aria-hidden />
              Desktop
            </Button>
            <Button
              type="button"
              size="sm"
              variant={device === "mobile" ? "secondary" : "ghost"}
              aria-pressed={device === "mobile"}
              onClick={() => setDevice("mobile")}
            >
              <Smartphone aria-hidden />
              Mobile
            </Button>
          </div>
        </div>
        <div className="flex flex-1 justify-center overflow-auto bg-muted/50 p-4">
          <article
            className={cn(
              "h-fit w-full rounded-lg border bg-background px-6 py-8 shadow-sm",
              device === "mobile" ? "max-w-[390px]" : "max-w-3xl md:px-12",
            )}
          >
            <h1 className="mb-4 font-serif text-4xl leading-tight">
              {title.trim() || "Untitled article"}
            </h1>
            {content.trim() ? (
              <div className="leading-7">
                <ArticlePreview content={content} />
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                Start writing to see the preview.
              </p>
            )}
          </article>
        </div>
      </DialogContent>
    </Dialog>
  );
}
