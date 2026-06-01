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
