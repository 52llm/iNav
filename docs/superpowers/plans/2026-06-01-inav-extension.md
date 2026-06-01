# inav Browser Extension Implementation Plan (Plan 3)

> **For agentic workers:** Implement task-by-task. Extension glue code is verified by static checks + a real backend round-trip + documented manual load-unpacked steps (no JS test runtime is introduced — see Verification rationale).

**Goal:** A Chrome MV3 extension that captures the current page (url, title, favicon, meta description, main text) and POSTs it to the user's inav backend with their token, showing quick feedback.

**Architecture & key decision — no build step.** Plain MV3 (manifest + vanilla JS), loaded via "Load unpacked". This deviates from the design doc's *default* of WXT/TypeScript (which was marked overridable): the extension is ~150 lines of glue, so a framework + bundler + node toolchain adds cost without payoff and conflicts with the project's single-binary, low-toolchain ethos. No `npm`, no bundler, no build. The tradeoff accepted: no static typing on the extension.

**Tech:** Chrome MV3, vanilla JS, `chrome.storage.local` for settings, `chrome.scripting.executeScript` for page extraction.

**Backend contract (already implemented, Plan 1):** `POST {baseURL}/api/bookmarks` with header `Authorization: Bearer <token>` and JSON body `{url, title, faviconUrl, excerpt, content}` → `200 {"id":N,"status":"pending"}`. Field names below MUST match exactly.

**Files (all new, under `extension/`):**
- `extension/manifest.json` — MV3 manifest
- `extension/popup.html` — popup UI (settings + capture button + status)
- `extension/popup.js` — settings load/save, capture flow, POST
- `extension/extract.js` — the pure extraction function injected into the page

**Verification rationale:** Browser-extension UI/glue has no headless runtime here (introducing jsdom/vitest would mean the npm toolchain this plan deliberately avoids). So correctness is established three ways: (1) `manifest.json` parses as valid JSON; (2) the exact POST payload the extension builds is replayed with `curl` against a running backend and accepted; (3) documented manual "Load unpacked" steps. The extraction function is written as a single pure function returning the payload object, so its field names can be eyeballed against the backend contract.

---

### Task 1: Manifest + popup shell

- [ ] **Step 1: Create `extension/manifest.json`:**
```json
{
  "manifest_version": 3,
  "name": "iNav 收藏",
  "version": "0.1.0",
  "description": "一键收藏当前页到 iNav",
  "permissions": ["activeTab", "scripting", "storage"],
  "host_permissions": ["http://*/*", "https://*/*"],
  "action": {
    "default_popup": "popup.html",
    "default_title": "收藏到 iNav"
  }
}
```
Rationale: `activeTab`+`scripting` allow extracting the current tab on click; broad `host_permissions` let the popup `fetch` the user-configured backend (any host) — acceptable for a personal self-hosted tool.

- [ ] **Step 2: Create `extension/popup.html`:**
```html
<!doctype html>
<html lang="zh">
<head>
  <meta charset="utf-8" />
  <style>
    body { font: 14px system-ui, sans-serif; width: 300px; margin: 0; padding: 12px; }
    h1 { font-size: 14px; margin: 0 0 8px; }
    button { width: 100%; padding: 8px; font-size: 14px; cursor: pointer; }
    .primary { background: #2563eb; color: #fff; border: none; border-radius: 6px; }
    .row { margin: 6px 0; }
    label { display: block; font-size: 12px; color: #555; margin-bottom: 2px; }
    input { width: 100%; box-sizing: border-box; padding: 6px; }
    #status { margin-top: 8px; min-height: 18px; font-size: 13px; }
    .ok { color: #16a34a; } .err { color: #dc2626; } .muted { color: #888; }
    details { margin-top: 10px; } summary { cursor: pointer; color: #555; font-size: 12px; }
  </style>
</head>
<body>
  <h1>iNav 收藏</h1>
  <button id="save" class="primary">收藏此页</button>
  <div id="status" class="muted"></div>
  <details id="settings">
    <summary>设置</summary>
    <div class="row">
      <label for="baseUrl">后端地址</label>
      <input id="baseUrl" placeholder="http://localhost:8080" />
    </div>
    <div class="row">
      <label for="token">Token</label>
      <input id="token" type="password" placeholder="INAV_TOKEN" />
    </div>
    <button id="saveSettings">保存设置</button>
  </details>
  <script src="extract.js"></script>
  <script src="popup.js"></script>
</body>
</html>
```

- [ ] **Step 3: Verify manifest is valid JSON**

Run: `python3 -c "import json; json.load(open('extension/manifest.json')); print('manifest OK')"`
Expected: `manifest OK`.

- [ ] **Step 4: Commit** — `git add extension/manifest.json extension/popup.html && git commit -m "feat(ext): manifest and popup shell"`

---

### Task 2: Page extraction function

`extract.js` defines one pure function on the global scope so it can be passed to `chrome.scripting.executeScript({func})`. It must return exactly the backend's expected fields.

- [ ] **Step 1: Create `extension/extract.js`:**
```js
// extractPageData runs in the context of the inspected page (injected via
// chrome.scripting.executeScript). It returns the inav bookmark payload.
// Field names MUST match the backend's POST /api/bookmarks contract.
function extractPageData() {
  function meta(name) {
    var el = document.querySelector(
      'meta[name="' + name + '"], meta[property="' + name + '"]'
    );
    return el ? (el.getAttribute("content") || "") : "";
  }

  var excerpt = meta("description") || meta("og:description") || "";

  var main = document.querySelector("article") || document.body;
  var content = ((main && main.innerText) || "").replace(/\s+/g, " ").trim();
  if (content.length > 8000) content = content.slice(0, 8000);

  var favicon = "";
  var link = document.querySelector('link[rel~="icon"]');
  if (link && link.getAttribute("href")) {
    try {
      favicon = new URL(link.getAttribute("href"), location.href).href;
    } catch (e) {
      favicon = "";
    }
  }
  if (!favicon) favicon = location.origin + "/favicon.ico";

  return {
    url: location.href,
    title: document.title,
    faviconUrl: favicon,
    excerpt: excerpt,
    content: content
  };
}
```

- [ ] **Step 2: Verify field names match backend contract**

The returned object keys (`url, title, faviconUrl, excerpt, content`) must equal the backend's `createBookmarkRequest` JSON tags. Confirm by reading `internal/api/handlers.go` (the `createBookmarkRequest` struct tags). They match.

- [ ] **Step 3: Commit** — `git add extension/extract.js && git commit -m "feat(ext): page extraction function"`

---

### Task 3: Popup logic (settings + capture + POST)

- [ ] **Step 1: Create `extension/popup.js`:**
```js
const els = {
  save: document.getElementById("save"),
  status: document.getElementById("status"),
  baseUrl: document.getElementById("baseUrl"),
  token: document.getElementById("token"),
  saveSettings: document.getElementById("saveSettings"),
  settings: document.getElementById("settings")
};

function setStatus(msg, cls) {
  els.status.textContent = msg;
  els.status.className = cls || "muted";
}

async function loadSettings() {
  const { baseUrl = "", token = "" } = await chrome.storage.local.get(["baseUrl", "token"]);
  els.baseUrl.value = baseUrl;
  els.token.value = token;
  return { baseUrl, token };
}

els.saveSettings.addEventListener("click", async () => {
  await chrome.storage.local.set({
    baseUrl: els.baseUrl.value.trim().replace(/\/+$/, ""),
    token: els.token.value.trim()
  });
  setStatus("设置已保存", "ok");
});

els.save.addEventListener("click", async () => {
  const { baseUrl, token } = await loadSettings();
  if (!baseUrl || !token) {
    setStatus("请先在设置里填后端地址和 Token", "err");
    els.settings.open = true;
    return;
  }
  setStatus("正在抓取页面…", "muted");

  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (!tab || !tab.id) {
    setStatus("无法获取当前标签页", "err");
    return;
  }

  let payload;
  try {
    const results = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: extractPageData
    });
    payload = results[0].result;
  } catch (e) {
    setStatus("无法读取此页面（可能是受限页面）", "err");
    return;
  }

  setStatus("正在收藏…", "muted");
  try {
    const resp = await fetch(baseUrl + "/api/bookmarks", {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: "Bearer " + token },
      body: JSON.stringify(payload)
    });
    if (resp.status === 401) {
      setStatus("Token 无效（401）", "err");
      return;
    }
    if (!resp.ok) {
      setStatus("收藏失败（HTTP " + resp.status + "）", "err");
      return;
    }
    setStatus("已收藏 ✓ 标签稍后自动生成", "ok");
  } catch (e) {
    setStatus("连接后端失败，请检查地址", "err");
  }
});

loadSettings();
```

- [ ] **Step 2: Backend round-trip verification (the real integration check)**

Build and run the backend, then replay the EXACT payload shape the extension sends:
```bash
go build -o inav . && INAV_TOKEN=dev INAV_DB_PATH=./ext-smoke.db ./inav &
PID=$!; sleep 1.2
curl -s -H "Authorization: Bearer dev" -H "Content-Type: application/json" \
  -XPOST localhost:8080/api/bookmarks \
  -d '{"url":"https://example.com","title":"Example","faviconUrl":"https://example.com/favicon.ico","excerpt":"desc","content":"body text"}'
echo ""
curl -s -H "Authorization: Bearer dev" localhost:8080/api/bookmarks
echo ""
kill $PID 2>/dev/null; rm -f ext-smoke.db ext-smoke.db-* inav
```
Expected: POST returns `{"id":1,"status":"pending"}`; GET returns the bookmark with that url/title. This proves the payload the extension builds is accepted by the backend.

- [ ] **Step 3: Commit** — `git add extension/popup.js && git commit -m "feat(ext): popup capture and POST logic"`

---

### Task 4: README for loading + manual test checklist

- [ ] **Step 1: Create `extension/README.md`:**
```markdown
# iNav 浏览器插件

无构建步骤的 Chrome MV3 插件，一键收藏当前页到你的 iNav 后端。

## 安装（开发/自用）
1. 打开 `chrome://extensions`
2. 右上角打开「开发者模式」
3. 点「加载已解压的扩展程序」，选择本 `extension/` 目录
4. 点击工具栏的 iNav 图标 → 展开「设置」→ 填后端地址（如 `http://localhost:8080`）和 Token（后端的 `INAV_TOKEN`）→ 保存

## 使用
在任意网页点击 iNav 图标 → 「收藏此页」。提示「已收藏 ✓」即成功，标签由后端异步生成。

## 受限页面
`chrome://`、Chrome 网上应用店等页面浏览器禁止注入脚本，无法收藏，会提示「无法读取此页面」。
```

- [ ] **Step 2: Manual test checklist (perform once in Chrome):**
  - Load unpacked from `extension/`.
  - Run backend locally (`INAV_TOKEN=dev ./inav`), set base URL + token in popup, save.
  - Visit a normal page, click 收藏此页 → expect 「已收藏 ✓」.
  - `curl -H "Authorization: Bearer dev" localhost:8080/api/bookmarks` → the page appears.
  - Wrong token → expect 「Token 无效（401）」.
  - On a `chrome://` page → expect 「无法读取此页面」.

- [ ] **Step 3: Commit** — `git add extension/README.md && git commit -m "docs(ext): install and manual test guide"`

---

## Self-Review

**Spec coverage (design §2 extension, §3.1 capture):**
- One-click capture of url/title/favicon/meta/main-text → Task 2 (`extractPageData`) ✓
- POST to backend with token, fast feedback → Task 3 ✓
- Token stored in extension storage, never the LLM key → Task 3 (`chrome.storage.local`) ✓
- Chrome MV3 → Task 1 ✓

**Deviation noted:** plain JS instead of WXT/TS (design default, overridable) — documented in header.

**Payload contract:** `extractPageData` returns `{url, title, faviconUrl, excerpt, content}` — matches `createBookmarkRequest` tags in `internal/api/handlers.go`. Verified by round-trip in Task 3 Step 2.

**Placeholder scan:** none.
