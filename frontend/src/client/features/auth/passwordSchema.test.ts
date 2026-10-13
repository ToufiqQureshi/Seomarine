import { describe, expect, it } from "vitest";
import {
  HOSTED_PASSWORD_MAX_LENGTH,
  HOSTED_PASSWORD_MIN_LENGTH,
} from "@/lib/auth-options";
import { passwordSchema } from "./passwordSchema";

describe("passwordSchema", () => {
  it("accepts values at both hosted password length boundaries", () => {
    expect(
      passwordSchema.safeParse("x".repeat(HOSTED_PASSWORD_MIN_LENGTH)).success,
    ).toBe(true);
    expect(
      passwordSchema.safeParse("x".repeat(HOSTED_PASSWORD_MAX_LENGTH)).success,
    ).toBe(true);
  });

  it("rejects empty, too-short, and too-long passwords", () => {
    for (const password of [
      "",
      "x".repeat(HOSTED_PASSWORD_MIN_LENGTH - 1),
      "x".repeat(HOSTED_PASSWORD_MAX_LENGTH + 1),
    ]) {
      expect(passwordSchema.safeParse(password).success).toBe(false);
    }
  });
});
