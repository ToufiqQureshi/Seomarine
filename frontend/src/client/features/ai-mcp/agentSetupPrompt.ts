import setupPrompt from "./agentSetupPrompt.md?raw";

export function getAgentSetupPrompt(origin: string) {
  return setupPrompt.trim().replaceAll("{{ORIGIN}}", origin);
}
