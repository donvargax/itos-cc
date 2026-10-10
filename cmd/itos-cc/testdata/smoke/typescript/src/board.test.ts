import { expect, test } from "vitest";
import { low } from "./board";

test("low", () => {
  expect(low(3)).toBe(true);
  expect(low(4)).toBe(false);
});
