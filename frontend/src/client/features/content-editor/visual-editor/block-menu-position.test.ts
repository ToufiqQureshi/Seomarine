import { describe, expect, it } from "vitest";
import { blockMenuPosition } from "./block-menu-position";

describe("block menu positioning", () => {
  it("flips above and shifts left when the trigger is near the lower-right edge", () => {
    expect(
      blockMenuPosition(
        { left: 900, top: 760, bottom: 780 },
        { width: 288, height: 400 },
        { width: 1024, height: 800 },
      ),
    ).toEqual({ left: 728, top: 352, maxHeight: 784 });
  });

  it("places the menu below when there is room and constrains its height", () => {
    expect(
      blockMenuPosition(
        { left: 12, top: 24, bottom: 40 },
        { width: 288, height: 300 },
        { width: 800, height: 500 },
      ),
    ).toEqual({ left: 12, top: 48, maxHeight: 484 });
  });
});
