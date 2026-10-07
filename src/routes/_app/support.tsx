import { createFileRoute, Link } from "@tanstack/react-router";
import { CopyButton } from "@/client/components/CopyButton";
import { PageHeader } from "@/client/components/PageHeader";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import { SUPPORT_EMAIL } from "@/client/lib/support";

export const Route = createFileRoute("/_app/support")({
  component: SupportPage,
});

function SupportPage() {
  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <PageHeader
          title="Help"
          description="Stuck, or have an idea? Write to us. A real person reads every email and we use your feedback to make Seomarine better."
        />

        <div className="space-y-3">
          <Card>
            <CardHeader>
              <CardTitle>
                <h2>Email</h2>
              </CardTitle>
              <CardDescription>
                Send ideas, problems, questions, or feedback directly.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <CopyButton
                value={SUPPORT_EMAIL}
                label={SUPPORT_EMAIL}
                successMessage="Email copied to clipboard"
                size="sm"
              />
            </CardContent>
          </Card>

          <Link to="/ai" className="group block">
            <Card className="transition-colors group-hover:border-primary/40">
              <CardHeader>
                <CardTitle>
                  <h2>Use Seomarine from your AI assistant</h2>
                </CardTitle>
                <CardDescription>
                  Connect ChatGPT, Claude Code or another assistant once, then
                  ask it about your site in plain words.
                </CardDescription>
              </CardHeader>
              <CardContent className="font-medium">
                Set up an assistant <span aria-hidden="true">&rarr;</span>
              </CardContent>
            </Card>
          </Link>
        </div>
      </div>
    </div>
  );
}
