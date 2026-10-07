import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@/client/components/ui/tabs";
import { Alert, AlertDescription } from "@/client/components/ui/alert";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import { PageHeader } from "@/client/components/PageHeader";
import { ShieldAlert } from "lucide-react";
import { getAuthMode } from "@/lib/auth-mode";
import { captureClientEvent } from "@/client/lib/posthog";
import { getAgentSetupPrompt } from "@/client/features/ai-mcp/agentSetupPrompt";
import { CopyButton } from "@/client/components/CopyButton";
import { AgentList } from "@/client/features/ai-mcp/AgentList";

const WORKFLOWS = [
  [
    "What should I do next for SEO?",
    "Explains where you stand and picks your next step.",
  ],
  [
    "Save my goals, competitors, and key pages",
    "Stores shared context your agent and the app both use.",
  ],
  [
    "Run an SEO audit of my site",
    "A one-page audit built around a single do-this-week action.",
  ],
  [
    "Find keywords worth targeting",
    "Finds keyword ideas from a few seed topics.",
  ],
  ["Group these keywords by intent", "Groups keywords and maps them to pages."],
  [
    "Who wins in my market and why?",
    "Maps your competitors and what they do well.",
  ],
  [
    "Study this competitor",
    "Looks at one competitor's keywords, content, and backlinks.",
  ],
  [
    "Find sites that might link to me",
    "Finds link prospects and drafts outreach.",
  ],
  [
    "Check my Google Business Profile",
    "Reviews your Maps visibility and local competitors.",
  ],
  ["Save that as a report", "Puts any of the above on your Reports page."],
];

const aiSearchSchema = z.object({
  // Active tab. Omitted for the default "setup" tab.
  tab: z.enum(["skills"]).optional().catch(undefined),
});

export const Route = createFileRoute("/_app/ai")({
  validateSearch: aiSearchSchema,
  component: AiPage,
});

function AiPage() {
  const { tab = "setup" } = Route.useSearch();
  const navigate = Route.useNavigate();
  const origin = window.location.origin;
  const mcpUrl = `${origin}/mcp`;
  const prompt = getAgentSetupPrompt(origin);

  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-7xl">
        <PageHeader
          title="Agent setup"
          description="Use Seomarine from the AI agent you already use. Connect it once, then ask it anything in plain words."
        />

        <Tabs
          value={tab}
          onValueChange={(value) =>
            void navigate({
              search: { tab: value === "skills" ? "skills" : undefined },
              replace: true,
            })
          }
          className="mt-8"
        >
          <TabsList variant="line">
            <TabsTrigger value="setup">Set up your agent</TabsTrigger>
            <TabsTrigger value="skills">What to ask</TabsTrigger>
          </TabsList>
          <TabsContent value="setup">
            <div className="mt-6 space-y-5">
              <Card size="lg">
                <CardHeader>
                  <CardTitle>
                    <h2>Set up your agent</h2>
                  </CardTitle>
                  <CardDescription>
                    Paste the setup prompt into your agent to connect Seomarine.
                    It will walk you through any steps it can&apos;t do on its
                    own.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <AgentList />
                  <div className="mt-5">
                    <CopyButton
                      variant="default"
                      size="lg"
                      value={prompt}
                      label="Copy setup prompt"
                      successMessage="Setup prompt copied"
                      onCopy={() => captureClientEvent("mcp:setup_prompt_copy")}
                    />
                  </div>
                </CardContent>
                <CardFooter className="text-muted-foreground">
                  <p>
                    Once connected, ask your agent &ldquo;What should I do next
                    for SEO?&rdquo; to get started.
                  </p>
                </CardFooter>
              </Card>
            </div>

            {getAuthMode(import.meta.env.AUTH_MODE) === "cloudflare_access" ? (
              <Alert variant="warning" className="mt-8">
                <ShieldAlert />
                <AlertDescription>
                  This instance is behind Cloudflare Access. MCP clients cannot
                  connect until Managed OAuth is enabled on your Access
                  application.
                </AlertDescription>
              </Alert>
            ) : null}

            <div className="mt-10 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-border pt-5 text-xs text-muted-foreground">
              <span>
                MCP server URL for this instance:{" "}
                <code className="font-mono text-foreground/80">{mcpUrl}</code>
              </span>
              <CopyButton
                value={mcpUrl}
                successMessage="MCP URL copied"
                onCopy={() => captureClientEvent("mcp:setup_url_copy")}
              />
            </div>
          </TabsContent>
          <TabsContent value="skills">
            <section className="mt-6">
              <p className="text-sm text-muted-foreground">
                Once your agent is connected, ask for any of these in your own
                words.
              </p>
              <ul className="mt-5 space-y-3 text-sm sm:space-y-2">
                {WORKFLOWS.map(([ask, blurb]) => (
                  <li
                    key={ask}
                    className="flex flex-col gap-0.5 sm:flex-row sm:gap-3"
                  >
                    <span className="shrink-0 font-medium text-foreground sm:w-80">
                      &ldquo;{ask}&rdquo;
                    </span>
                    <span className="text-muted-foreground">{blurb}</span>
                  </li>
                ))}
              </ul>
            </section>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
