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
