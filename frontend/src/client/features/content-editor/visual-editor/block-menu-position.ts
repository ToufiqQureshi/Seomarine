export function blockMenuPosition(
  anchor: Pick<DOMRect, "left" | "bottom" | "top">,
  menu: Pick<DOMRect, "width" | "height">,
  viewport: { width: number; height: number },
) {
  const maxHeight = Math.max(120, viewport.height - 16);
  const height = Math.min(menu.height, maxHeight);
  const left = Math.max(
    8,
    Math.min(anchor.left, viewport.width - menu.width - 8),
  );
  const below = anchor.bottom + height + 8 <= viewport.height;
  const top = below ? anchor.bottom + 8 : Math.max(8, anchor.top - height - 8);
  return { left, top, maxHeight };
}
