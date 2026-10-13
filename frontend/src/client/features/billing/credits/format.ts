const credits = new Intl.NumberFormat("en-IN");

/** A credit count as the customer reads it, e.g. "10,000". */
export function formatCredits(value: number) {
  return credits.format(Math.max(0, Math.round(value)));
}
