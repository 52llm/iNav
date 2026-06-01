# inav Nav Site + CRUD Admin Implementation Plan (Plan 4)

> **For agentic workers:** Implement task-by-task. Pure logic (the API client) is unit-tested with vitest; the UI is verified by a successful production build that the Go binary embeds and serves (assets return 200), plus a documented manual checklist. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A single-page nav site that displays bookmarks as a tag-filterable, searchable card grid (click to open), plus CRUD admin actions (edit a bookmark's tags, delete, retag) and tag management (rename, merge) — all talking to the existing backend with a token.

**Architecture:**
- React + Vite + TypeScript + Tailwind v4 SPA in `web/`.
- **Single page, no client-side router** (views toggle by state) — so the Go `FileServer` needs no SPA fallback.
- Vite builds into `internal/web/dist/` (where `embed.FS` reads). **The built output is committed**, so `go build`/`go install` produces a binary with the real UI without node. A `Makefile` rebuilds the frontend; only frontend changes need node. (Tradeoff: build artifacts churn in git — accepted for deployment simplicity.)
- Token kept in `localStorage`; sent as `Authorization: Bearer <token>` on every request. A `401` surfaces the login screen.

**Backend API (implemented in Plans 1–2):**
| Method | Path | Body | Returns |
|---|---|---|---|
| GET | `/api/bookmarks?tag=&q=` | – | `[{id,url,title,faviconUrl,summary,status,tags[]}]` |
| GET | `/api/tags` | – | `["name", …]` |
| PATCH | `/api/bookmarks/{id}/tags` | `{add[],remove[]}` | `{tags[]}` |
| POST | `/api/bookmarks/{id}/retag` | – | `{id,status}` |
| DELETE | `/api/bookmarks/{id}` | – | 204 |
| POST | `/api/tags/rename` | `{oldName,newName}` | `{ok}` |
| POST | `/api/tags/merge` | `{sources[],target}` | `{affected}` |

**File structure (under `web/`):**
```
web/
├── package.json, vite.config.ts, tsconfig.json, index.html
└── src/
    ├── main.tsx            # React entry
    ├── App.tsx             # layout + top-level state (filter, query, view)
    ├── index.css           # @import "tailwindcss"
    ├── types.ts            # Bookmark type
    ├── api.ts              # typed client (token from localStorage)
    ├── api.test.ts         # vitest unit tests for the client
    ├── useAuth.ts          # token get/set/clear + Unauthorized handling
    └── components/
        ├── Login.tsx       # token entry
        ├── SearchBar.tsx
        ├── TagSidebar.tsx  # tag list, active-tag filter
        ├── BookmarkCard.tsx
        ├── BookmarkGrid.tsx
        ├── TagEditor.tsx   # add/remove tags on one bookmark
        └── TagManager.tsx  # rename / merge tags
```

---

### Task 1: Scaffold Vite + React + TS + Tailwind, wire build output

**Files:** `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`, `web/src/index.css`; modify root `Makefile` (new) and `.gitignore`.

- [ ] **Step 1: Create the Vite project files**

`web/package.json`:
```json
{
  "name": "inav-web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "test": "vitest run"
  },
  "dependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@tailwindcss/vite": "^4.0.0",
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^5.0.0",
    "tailwindcss": "^4.0.0",
    "typescript": "^5.7.0",
    "vite": "^7.0.0",
    "vitest": "^3.0.0"
  }
}
```

`web/vite.config.ts`:
```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "./",
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
});
```

`web/tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true
  },
  "include": ["src"]
}
```

`web/index.html`:
```html
<!doctype html>
<html lang="zh">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>iNav</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

`web/src/index.css`:
```css
@import "tailwindcss";
```

`web/src/main.tsx`:
```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);
```

`web/src/App.tsx` (minimal placeholder; fleshed out in Task 4):
```tsx
export default function App() {
  return <div className="p-8 text-xl">iNav</div>;
}
```

- [ ] **Step 2: Update `.gitignore`** — replace the `/web/dist/` line with node/build ignores for the source project (the committed embed output under `internal/web/dist/` stays tracked):
```
/inav
/inav.db
/inav.db-*
*.test
/web/node_modules/
/web/.vite/
```

- [ ] **Step 3: Create root `Makefile`:**
```makefile
.PHONY: web build test

web:
	cd web && npm install && npm run build

build: web
	go build -o inav .

test:
	go test ./...
	cd web && npm run test
```

- [ ] **Step 4: Install deps and build**

Run:
```bash
cd web && npm install && npm run build
```
Expected: `internal/web/dist/` now contains `index.html` + `assets/`. (`emptyOutDir` removes the Plan 1 placeholder and writes the real build.)

- [ ] **Step 5: Verify the Go binary embeds and serves the built app**

Run (from repo root):
```bash
go build -o inav . && INAV_TOKEN=dev INAV_DB_PATH=./n.db INAV_PUBLIC_READ=true ./inav &
PID=$!; sleep 1
curl -s -o /dev/null -w "index=%{http_code}\n" localhost:8080/
ASSET=$(grep -o '/assets/[^"]*\.js' internal/web/dist/index.html | head -1)
curl -s -o /dev/null -w "asset=%{http_code}\n" "localhost:8080$ASSET" 2>/dev/null || true
kill $PID; rm -f n.db n.db-* inav
```
Expected: `index=200`; asset 200 (path depends on base — if base `./` the asset path is relative, adjust grep accordingly; the key check is `index=200`).

- [ ] **Step 6: Commit** — `git add web/ internal/web/dist/ Makefile .gitignore && git commit -m "feat(web): scaffold react+vite+tailwind, embed build output"`

---

### Task 2: Types + API client (+ vitest)

**Files:** `web/src/types.ts`, `web/src/api.ts`, `web/src/api.test.ts`, `web/src/useAuth.ts`

- [ ] **Step 1: Write the failing test** — `web/src/api.test.ts`:
```ts
import { describe, it, expect, vi, beforeEach } from "vitest";
import { Api, Unauthorized } from "./api";

beforeEach(() => {
  localStorage.clear();
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
    const f = mockFetch(200, [{ id: 1, url: "u", title: "t", faviconUrl: "", summary: "", status: "tagged", tags: ["Go"] }]);
    const api = new Api("http://x", f);
    const out = await api.listBookmarks({ tag: "Go", q: "hi" });
    expect(out).toHaveLength(1);
    const [url, init] = (f as any).mock.calls[0];
    expect(url).toContain("/api/bookmarks?");
    expect(url).toContain("tag=Go");
    expect(url).toContain("q=hi");
    expect((init.headers as any).Authorization).toBe("Bearer secret");
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
    const [url, init] = (f as any).mock.calls[0];
    expect(url).toBe("http://x/api/tags/merge");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({ sources: ["a", "b"], target: "c" });
  });
});
```

- [ ] **Step 2: Run, verify fail** — `cd web && npx vitest run src/api.test.ts` → FAIL (no `./api`).

- [ ] **Step 3: Implement**

`web/src/types.ts`:
```ts
export interface Bookmark {
  id: number;
  url: string;
  title: string;
  faviconUrl: string;
  summary: string;
  status: "pending" | "tagged" | "failed";
  tags: string[];
}
```

`web/src/api.ts`:
```ts
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
    return (await this.req(`/api/bookmarks/${id}/tags`, {
      method: "PATCH",
      body: JSON.stringify({ add, remove }),
    })).json();
  }

  async retag(id: number): Promise<void> {
    await this.req(`/api/bookmarks/${id}/retag`, { method: "POST" });
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
```

`web/src/useAuth.ts`:
```ts
import { useState, useCallback } from "react";

const TOKEN_KEY = "inav_token";

export function useAuth() {
  const [token, setTokenState] = useState<string>(() => localStorage.getItem(TOKEN_KEY) ?? "");

  const setToken = useCallback((t: string) => {
    localStorage.setItem(TOKEN_KEY, t);
    setTokenState(t);
  }, []);

  const clear = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY);
    setTokenState("");
  }, []);

  return { token, hasToken: token.length > 0, setToken, clear };
}
```

- [ ] **Step 4: Add vitest dom env for localStorage** — vitest needs a DOM for `localStorage`. Add to `web/vite.config.ts` a `test` block:
```ts
  // add inside defineConfig({...})
  test: { environment: "jsdom" },
```
and install jsdom: `cd web && npm install -D jsdom`. (Add `/// <reference types="vitest/config" />` at the top of vite.config.ts if TS complains about the `test` key.)

- [ ] **Step 5: Run, verify pass** — `cd web && npm run test` → PASS.

- [ ] **Step 6: Commit** — `git add web/ internal/web/dist/ && git commit -m "feat(web): typed api client + auth, with tests"` (dist unchanged here, but include if regenerated).

---

### Task 3: Login gate

**Files:** `web/src/components/Login.tsx`; modify `web/src/App.tsx`

- [ ] **Step 1: Create `web/src/components/Login.tsx`:**
```tsx
import { useState } from "react";

export default function Login({ onSubmit }: { onSubmit: (token: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <form
        className="bg-white p-6 rounded-lg shadow w-80 space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (value.trim()) onSubmit(value.trim());
        }}
      >
        <h1 className="text-lg font-semibold">登录 iNav</h1>
        <input
          className="w-full border rounded px-3 py-2"
          type="password"
          placeholder="Token"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          autoFocus
        />
        <button className="w-full bg-blue-600 text-white rounded py-2">进入</button>
      </form>
    </div>
  );
}
```

- [ ] **Step 2: Wire into `App.tsx`** (interim — full layout in Task 4):
```tsx
import { useAuth } from "./useAuth";
import Login from "./components/Login";

export default function App() {
  const { hasToken, setToken } = useAuth();
  if (!hasToken) return <Login onSubmit={setToken} />;
  return <div className="p-8">已登录（导航站内容见下一步）</div>;
}
```

- [ ] **Step 3: Verify build** — `cd web && npm run build` → succeeds, dist regenerated.
- [ ] **Step 4: Commit** — `git add web/ internal/web/dist/ && git commit -m "feat(web): token login gate"`

---

### Task 4: Nav site core — grid, tag sidebar, search

**Files:** `web/src/components/{SearchBar,TagSidebar,BookmarkCard,BookmarkGrid}.tsx`; rewrite `web/src/App.tsx`

- [ ] **Step 1: `web/src/components/BookmarkCard.tsx`:**
```tsx
import type { Bookmark } from "../types";

const statusBadge: Record<Bookmark["status"], string> = {
  pending: "bg-amber-100 text-amber-700",
  tagged: "bg-green-100 text-green-700",
  failed: "bg-red-100 text-red-700",
};

export default function BookmarkCard({
  b,
  onTagClick,
  children,
}: {
  b: Bookmark;
  onTagClick: (tag: string) => void;
  children?: React.ReactNode;
}) {
  return (
    <div className="bg-white rounded-lg shadow-sm border p-4 flex flex-col gap-2">
      <a href={b.url} target="_blank" rel="noreferrer" className="flex items-center gap-2 font-medium hover:underline">
        {b.faviconUrl && <img src={b.faviconUrl} alt="" className="w-4 h-4" onError={(e) => (e.currentTarget.style.display = "none")} />}
        <span className="truncate">{b.title || b.url}</span>
      </a>
      {b.summary && <p className="text-sm text-gray-600 line-clamp-2">{b.summary}</p>}
      <div className="flex flex-wrap gap-1">
        {b.tags.map((t) => (
          <button key={t} onClick={() => onTagClick(t)} className="text-xs bg-gray-100 hover:bg-gray-200 rounded px-2 py-0.5">
            {t}
          </button>
        ))}
        <span className={`text-xs rounded px-2 py-0.5 ${statusBadge[b.status]}`}>{b.status}</span>
      </div>
      {children}
    </div>
  );
}
```

- [ ] **Step 2: `web/src/components/BookmarkGrid.tsx`:**
```tsx
import type { Bookmark } from "../types";
import BookmarkCard from "./BookmarkCard";

export default function BookmarkGrid({
  bookmarks,
  onTagClick,
  renderActions,
}: {
  bookmarks: Bookmark[];
  onTagClick: (tag: string) => void;
  renderActions: (b: Bookmark) => React.ReactNode;
}) {
  if (bookmarks.length === 0) return <p className="text-gray-500">没有收藏</p>;
  return (
    <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
      {bookmarks.map((b) => (
        <BookmarkCard key={b.id} b={b} onTagClick={onTagClick}>
          {renderActions(b)}
        </BookmarkCard>
      ))}
    </div>
  );
}
```

- [ ] **Step 3: `web/src/components/SearchBar.tsx`:**
```tsx
export default function SearchBar({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <input
      className="border rounded px-3 py-2 w-full max-w-md"
      placeholder="搜索标题 / 网址 / 摘要…"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}
```

- [ ] **Step 4: `web/src/components/TagSidebar.tsx`:**
```tsx
export default function TagSidebar({
  tags,
  active,
  onSelect,
}: {
  tags: string[];
  active: string;
  onSelect: (tag: string) => void;
}) {
  return (
    <aside className="w-44 shrink-0 space-y-1">
      <button
        onClick={() => onSelect("")}
        className={`block w-full text-left px-2 py-1 rounded ${active === "" ? "bg-blue-100 text-blue-700" : "hover:bg-gray-100"}`}
      >
        全部
      </button>
      {tags.map((t) => (
        <button
          key={t}
          onClick={() => onSelect(t)}
          className={`block w-full text-left px-2 py-1 rounded truncate ${active === t ? "bg-blue-100 text-blue-700" : "hover:bg-gray-100"}`}
        >
          {t}
        </button>
      ))}
    </aside>
  );
}
```

- [ ] **Step 5: Rewrite `web/src/App.tsx`** to load data and compose the layout. Search is debounced; tag filter and search re-query the backend. Includes a logout button and a "管理标签" toggle (TagManager comes in Task 5; import + button wired then). Per-card actions (TagEditor, delete, retag) come in Task 5 via `renderActions`.
```tsx
import { useCallback, useEffect, useState } from "react";
import { api } from "./api";
import { Unauthorized } from "./api";
import { useAuth } from "./useAuth";
import type { Bookmark } from "./types";
import Login from "./components/Login";
import SearchBar from "./components/SearchBar";
import TagSidebar from "./components/TagSidebar";
import BookmarkGrid from "./components/BookmarkGrid";

export default function App() {
  const { hasToken, setToken, clear } = useAuth();
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [tags, setTags] = useState<string[]>([]);
  const [activeTag, setActiveTag] = useState("");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");

  const reload = useCallback(async () => {
    try {
      const [bs, ts] = await Promise.all([
        api.listBookmarks({ tag: activeTag, q: query }),
        api.listTags(),
      ]);
      setBookmarks(bs);
      setTags(ts);
      setError("");
    } catch (e) {
      if (e instanceof Unauthorized) clear();
      else setError("加载失败");
    }
  }, [activeTag, query, clear]);

  useEffect(() => {
    if (!hasToken) return;
    const t = setTimeout(reload, 250); // debounce query changes
    return () => clearTimeout(t);
  }, [hasToken, reload]);

  if (!hasToken) return <Login onSubmit={setToken} />;

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white border-b px-6 py-3 flex items-center gap-4">
        <h1 className="text-lg font-semibold">iNav</h1>
        <SearchBar value={query} onChange={setQuery} />
        <button onClick={clear} className="ml-auto text-sm text-gray-500 hover:text-gray-800">退出</button>
      </header>
      {error && <div className="px-6 py-2 text-red-600">{error}</div>}
      <div className="flex gap-6 p-6">
        <TagSidebar tags={tags} active={activeTag} onSelect={setActiveTag} />
        <main className="flex-1">
          <BookmarkGrid
            bookmarks={bookmarks}
            onTagClick={setActiveTag}
            renderActions={() => null}
          />
        </main>
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Build + serve verification** — `cd web && npm run build` then run the binary with seeded data:
```bash
go build -o inav . && INAV_TOKEN=dev INAV_DB_PATH=./n.db ./inav &
PID=$!; sleep 1
curl -s -H "Authorization: Bearer dev" -H "Content-Type: application/json" -XPOST localhost:8080/api/bookmarks -d '{"url":"https://go.dev","title":"Go","content":"x"}' >/dev/null
curl -s -H "Authorization: Bearer dev" "localhost:8080/api/bookmarks?q=go" | head -c 200; echo ""
curl -s -o /dev/null -w "index=%{http_code}\n" localhost:8080/
kill $PID; rm -f n.db n.db-* inav
```
Expected: bookmark JSON returned; `index=200`.

- [ ] **Step 7: Commit** — `git add web/ internal/web/dist/ && git commit -m "feat(web): nav site grid, tag filter, search"`

---

### Task 5: CRUD admin — per-bookmark actions + tag manager

**Files:** `web/src/components/{TagEditor,TagManager}.tsx`; modify `web/src/App.tsx`

- [ ] **Step 1: `web/src/components/TagEditor.tsx`** (add/remove tags + retag + delete for one bookmark):
```tsx
import { useState } from "react";
import type { Bookmark } from "../types";
import { api } from "../api";

export default function TagEditor({ b, onChanged }: { b: Bookmark; onChanged: () => void }) {
  const [adding, setAdding] = useState("");
  const [busy, setBusy] = useState(false);

  async function run(fn: () => Promise<unknown>) {
    setBusy(true);
    try {
      await fn();
      onChanged();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="border-t pt-2 mt-1 flex flex-wrap items-center gap-2 text-xs">
      <input
        className="border rounded px-1 py-0.5 w-24"
        placeholder="加标签"
        value={adding}
        onChange={(e) => setAdding(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && adding.trim()) {
            run(() => api.patchTags(b.id, [adding.trim()], []));
            setAdding("");
          }
        }}
      />
      {b.tags.map((t) => (
        <button key={t} disabled={busy} onClick={() => run(() => api.patchTags(b.id, [], [t]))} className="bg-gray-100 rounded px-1 hover:bg-red-100" title="移除">
          {t} ✕
        </button>
      ))}
      <button disabled={busy} onClick={() => run(() => api.retag(b.id))} className="text-blue-600 hover:underline">重打标</button>
      <button disabled={busy} onClick={() => { if (confirm("删除这条收藏？")) run(() => api.deleteBookmark(b.id)); }} className="text-red-600 hover:underline">删除</button>
    </div>
  );
}
```

- [ ] **Step 2: `web/src/components/TagManager.tsx`** (rename + merge):
```tsx
import { useState } from "react";
import { api } from "../api";

export default function TagManager({ tags, onChanged }: { tags: string[]; onChanged: () => void }) {
  const [oldName, setOldName] = useState("");
  const [newName, setNewName] = useState("");
  const [sources, setSources] = useState("");
  const [target, setTarget] = useState("");
  const [msg, setMsg] = useState("");

  async function rename() {
    if (!oldName || !newName) return;
    await api.renameTag(oldName, newName);
    setMsg(`已重命名 ${oldName} → ${newName}`);
    onChanged();
  }
  async function merge() {
    const src = sources.split(",").map((s) => s.trim()).filter(Boolean);
    if (src.length === 0 || !target) return;
    await api.mergeTags(src, target);
    setMsg(`已合并 ${src.join("、")} → ${target}`);
    onChanged();
  }

  return (
    <div className="bg-white border rounded-lg p-4 space-y-4 text-sm">
      <div>
        <h3 className="font-medium mb-2">重命名标签</h3>
        <div className="flex gap-2">
          <input list="taglist" className="border rounded px-2 py-1" placeholder="原标签" value={oldName} onChange={(e) => setOldName(e.target.value)} />
          <input className="border rounded px-2 py-1" placeholder="新名称" value={newName} onChange={(e) => setNewName(e.target.value)} />
          <button onClick={rename} className="bg-blue-600 text-white rounded px-3">重命名</button>
        </div>
      </div>
      <div>
        <h3 className="font-medium mb-2">合并标签</h3>
        <div className="flex gap-2">
          <input className="border rounded px-2 py-1 flex-1" placeholder="来源（逗号分隔）" value={sources} onChange={(e) => setSources(e.target.value)} />
          <input list="taglist" className="border rounded px-2 py-1" placeholder="合并到" value={target} onChange={(e) => setTarget(e.target.value)} />
          <button onClick={merge} className="bg-blue-600 text-white rounded px-3">合并</button>
        </div>
      </div>
      <datalist id="taglist">{tags.map((t) => <option key={t} value={t} />)}</datalist>
      {msg && <p className="text-green-600">{msg}</p>}
    </div>
  );
}
```

- [ ] **Step 3: Wire into `App.tsx`** — add a `manage` boolean state + a toggle button in the header; render `TagManager` above the grid when on; pass `renderActions={(b) => <TagEditor b={b} onChanged={reload} />}` to the grid; import both components.
```tsx
// add imports
import TagEditor from "./components/TagEditor";
import TagManager from "./components/TagManager";
// add state: const [manage, setManage] = useState(false);
// header button (before 退出): <button onClick={() => setManage(m => !m)} className="text-sm text-gray-600 hover:text-gray-900">{manage ? "完成" : "管理标签"}</button>
// in <main>, before grid: {manage && <div className="mb-4"><TagManager tags={tags} onChanged={reload} /></div>}
// grid prop: renderActions={(b) => <TagEditor b={b} onChanged={reload} />}
```

- [ ] **Step 4: Build + full verification** — `make build` (frontend + go), then a round-trip exercising the management flow:
```bash
make build
INAV_TOKEN=dev INAV_DB_PATH=./n.db ./inav &
PID=$!; sleep 1
A="Authorization: Bearer dev"; J="Content-Type: application/json"
curl -s -H "$A" -H "$J" -XPOST localhost:8080/api/bookmarks -d '{"url":"https://go.dev","title":"Go","content":"x"}' >/dev/null
curl -s -H "$A" -H "$J" -XPATCH localhost:8080/api/bookmarks/1/tags -d '{"add":["lang"]}' >/dev/null
curl -s -H "$A" localhost:8080/api/tags; echo " <- tags"
curl -s -o /dev/null -w "index=%{http_code}\n" localhost:8080/
kill $PID; rm -f n.db n.db-* inav
```
Expected: tags include `lang`; `index=200`.

- [ ] **Step 5: Manual UI checklist (perform once in a browser):**
  - `make build`, run backend, open `http://localhost:8080`, enter token → grid loads.
  - Capture a few pages with the extension (Plan 3) → cards appear, status `pending`→`tagged` once an LLM is configured.
  - Click a tag chip / sidebar tag → grid filters; search box narrows results.
  - On a card: add a tag (Enter), remove a tag (✕), 重打标, 删除 → grid updates.
  - 管理标签 → rename a tag, merge two tags → grid/sidebar reflect changes.

- [ ] **Step 6: Commit** — `git add web/ internal/web/dist/ && git commit -m "feat(web): crud admin — per-bookmark actions and tag manager"`

---

## Self-Review

**Spec coverage (design §3.3 browse, §3.4 CRUD, §11 v1 nav site):**
- Card grid with favicon/title/summary, click-to-open → Task 4 (BookmarkCard/Grid) ✓
- Tag filter + search → Task 4 (TagSidebar/SearchBar/App query) ✓
- Status surfaced (pending/tagged/failed) + retry via retag → Task 4 badge, Task 5 retag ✓
- CRUD: edit tags, delete, retag → Task 5 (TagEditor) ✓
- Tag management: rename, merge → Task 5 (TagManager) ✓
- Token auth, 401 → login → Tasks 2,3 (Unauthorized → clear) ✓
- Single binary deploy (embedded, committed dist) → Task 1 ✓

**Deferred (consistent with design §11):** LLM tidy assistant (v1.1), screenshots, split_tag UI.

**Verification depth:** API client unit-tested (vitest); UI verified by production build embedded+served (HTTP 200) + management round-trips via curl + documented manual browser checklist. (No headless React E2E — honest limitation.)

**Type/contract consistency:** `Bookmark` fields match backend JSON tags; `Api` method paths/bodies match the Plan 1–2 endpoints table; `renderActions`/`onTagClick`/`onChanged` prop names consistent across App↔Grid↔Card↔TagEditor.
