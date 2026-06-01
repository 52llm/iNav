import { describe, it, expect, vi, beforeEach } from "vitest";
import { Api, Unauthorized } from "./api";

// A minimal in-memory localStorage, independent of the test environment
// (Node 26's built-in localStorage global is unavailable without a backing file).
beforeEach(() => {
  const store = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
    clear: () => store.clear(),
  });
  localStorage.setItem("inav_token", "secret");
});

function mockFetch(status: number, body: unknown) {
  return vi.fn(async () => ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  })) as unknown as typeof fetch;
}

describe("Api", () => {
  it("lists bookmarks with tag+q and auth header", async () => {
    const f = mockFetch(200, [
      { id: 1, url: "u", title: "t", faviconUrl: "", summary: "", status: "tagged", tags: ["Go"] },
    ]);
    const api = new Api("http://x", f);
    const out = await api.listBookmarks({ tag: "Go", q: "hi" });
    expect(out).toHaveLength(1);
    const [url, init] = (f as unknown as { mock: { calls: [string, RequestInit][] } }).mock.calls[0];
    expect(url).toContain("/api/bookmarks?");
    expect(url).toContain("tag=Go");
    expect(url).toContain("q=hi");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer secret");
  });

  it("throws Unauthorized on 401", async () => {
    const f = mockFetch(401, {});
    const api = new Api("http://x", f);
    await expect(api.listTags()).rejects.toBeInstanceOf(Unauthorized);
  });

  it("merges tags via POST", async () => {
    const f = mockFetch(200, { affected: 2 });
    const api = new Api("http://x", f);
    await api.mergeTags(["a", "b"], "c");
    const [url, init] = (f as unknown as { mock: { calls: [string, RequestInit][] } }).mock.calls[0];
    expect(url).toBe("http://x/api/tags/merge");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ sources: ["a", "b"], target: "c" });
  });
});
