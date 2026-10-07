import { describe, expect, it } from "vitest";
import { getAgentSetupPrompt } from "./agentSetupPrompt";

describe("agent setup prompt", () => {
  it("points every link at the current instance", () => {
    const prompt = getAgentSetupPrompt("https://seo.example.com");
    expect(prompt).toContain("https://seo.example.com/mcp");
    expect(prompt).toContain("https://seo.example.com/settings");
    expect(prompt).not.toContain("{{ORIGIN}}");
    expect(prompt).not.toContain("seomarine.com");
  });
});
