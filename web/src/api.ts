import type { Bookmark } from "./types";

export class Unauthorized extends Error {}

const TOKEN_KEY = "inav_token";

export class Api {
  constructor(
    private baseUrl: string = "",
    private fetcher: typeof fetch = fetch.bind(globalThis)
  ) {}

  private token(): string {
    return localStorage.getItem(TOKEN_KEY) ?? "";
  }

  private async req(path: string, init: RequestInit = {}): Promise<Response> {
    const resp = await this.fetcher(this.baseUrl + path, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer " + this.token(),
        ...(init.headers ?? {}),
      },
    });
    if (resp.status === 401) throw new Unauthorized("unauthorized");
    if (!resp.ok) throw new Error("HTTP " + resp.status);
    return resp;
  }

  async listBookmarks(params: { tag?: string; q?: string } = {}): Promise<Bookmark[]> {
    const qs = new URLSearchParams();
    if (params.tag) qs.set("tag", params.tag);
    if (params.q) qs.set("q", params.q);
    const resp = await this.req("/api/bookmarks?" + qs.toString());
    return resp.json();
  }

  async listTags(): Promise<string[]> {
    return (await this.req("/api/tags")).json();
  }

  async patchTags(id: number, add: string[], remove: string[]): Promise<{ tags: string[] }> {
    return (
      await this.req(`/api/bookmarks/${id}/tags`, {
        method: "PATCH",
        body: JSON.stringify({ add, remove }),
      })
    ).json();
  }

  async retag(id: number): Promise<void> {
    await this.req(`/api/bookmarks/${id}/retag`, { method: "POST" });
  }

  async retagAll(): Promise<{ queued: number }> {
    return (await this.req("/api/bookmarks/retag-all", { method: "POST" })).json();
  }

  async clearTags(): Promise<{ cleared: number }> {
    return (await this.req("/api/tags/clear", { method: "POST" })).json();
  }

  async deleteBookmark(id: number): Promise<void> {
    await this.req(`/api/bookmarks/${id}`, { method: "DELETE" });
  }

  async renameTag(oldName: string, newName: string): Promise<void> {
    await this.req("/api/tags/rename", {
      method: "POST",
      body: JSON.stringify({ oldName, newName }),
    });
  }

  async mergeTags(sources: string[], target: string): Promise<void> {
    await this.req("/api/tags/merge", {
      method: "POST",
      body: JSON.stringify({ sources, target }),
    });
  }
}

export const api = new Api();
