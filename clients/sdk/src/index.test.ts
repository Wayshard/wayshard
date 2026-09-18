import { describe, expect, test } from "bun:test";
import { WayshardClient } from "./index";

describe("WayshardClient", () => {
  test("constructs with base url", () => {
    const c = new WayshardClient("http://127.0.0.1:7420", "wsd_test");
    expect(c.baseUrl).toContain("7420");
    expect(c.token).toBe("wsd_test");
  });
});
