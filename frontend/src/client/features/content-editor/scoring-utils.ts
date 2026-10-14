export type ScoreCheck = {
  id: string;
  label: string;
  detail: string;
  passed: boolean;
  weight: number;
  earned: number;
};

export const makeScoreCheck = (
  id: string,
  label: string,
  detail: string,
  weight: number,
  earned: number,
): ScoreCheck => ({
  id,
  label,
  detail,
  weight,
  earned: Math.max(0, Math.min(weight, earned)),
  passed: earned >= weight,
});

export const sumEarnedPoints = (checks: ScoreCheck[]) =>
  Math.round(checks.reduce((sum, check) => sum + check.earned, 0));
