export const MAX_TARGET_TOPICS = 5;

export function parseTargetTopics(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(/[\n,]+/u)
    .map((part) => part.trim())
    .filter((part) => {
      const key = part.toLocaleLowerCase();
      if (!part || seen.has(key) || seen.size >= MAX_TARGET_TOPICS)
        return false;
      seen.add(key);
      return true;
    });
}

export const primaryTargetTopic = (value: string) =>
  parseTargetTopics(value)[0] ?? "";
