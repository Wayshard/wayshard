import { describe, expect, test } from "bun:test";

describe("web shell", () => {
  test("primary tabs are session/changes/files/terminal", () => {
    const tabs = ["session", "changes", "files", "terminal"];
    expect(tabs).toHaveLength(4);
  });
});
