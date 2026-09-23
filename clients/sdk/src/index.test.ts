import { describe, expect, test } from "bun:test";
import { WayshardClient } from "./index";

describe("WayshardClient", () => {
  test("constructs with base url", () => {
    const c = new WayshardClient("http://127.0.0.1:7420", "wsd_test");
    expect(c.baseUrl).toContain("7420");
    expect(c.token).toBe("wsd_test");
  });
});

describe("eventURL", () => {
  test("browser client (no token) does not request a ticket", async () => {
    const orig = globalThis.fetch;
    let called = 0;
    globalThis.fetch = (async () => {
      called++;
      return new Response("{}", { status: 200 });
    }) as typeof fetch;
    try {
      const c = new WayshardClient("http://127.0.0.1:7420");
      const url = await c.eventURL(7);
      expect(called).toBe(0);
      expect(url).toContain("lastEventSeq=7");
      expect(url).not.toContain("ticket=");
    } finally {
      globalThis.fetch = orig;
    }
  });

  test("native client (token) attaches a single-use ticket", async () => {
    const orig = globalThis.fetch;
    let seen = "";
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      seen = String(input);
      return new Response(JSON.stringify({ ticket: "TICK123" }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }) as typeof fetch;
    try {
      const c = new WayshardClient("http://127.0.0.1:7420", "wsd_test");
      const url = await c.eventURL(3);
      expect(seen).toContain("/v1/ws/ticket");
      expect(url).toContain("ticket=TICK123");
    } finally {
      globalThis.fetch = orig;
    }
  });
});
