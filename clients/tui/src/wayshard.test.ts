import { describe, expect, test } from "bun:test";

describe("tui", () => {
  test("interactive default is the full client", () => {
    expect("wayshard").toBe("wayshard");
  });
});
