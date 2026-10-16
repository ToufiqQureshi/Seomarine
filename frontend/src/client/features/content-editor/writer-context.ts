export function writerContext(
  content: string,
  selection: string,
  currentBlock: string,
) {
  if (selection.trim()) return selection.trim().slice(0, 5_000);
  const block = currentBlock.trim();
  if (!block) return content.slice(0, 5_000);
  const lines = content.split("\n");
  const at = lines.findIndex((line) => line.includes(block));
  if (at < 0) return block.slice(0, 5_000);
  let start = at;
  while (start > 0 && !/^#{1,3}\s/u.test(lines[start] ?? "")) start--;
  let end = at + 1;
  while (end < lines.length && !/^#{1,3}\s/u.test(lines[end] ?? "")) end++;
  return lines.slice(start, end).join("\n").trim().slice(0, 5_000);
}
