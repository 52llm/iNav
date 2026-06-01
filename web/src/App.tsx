import { useCallback, useEffect, useState } from "react";
import { api, Unauthorized } from "./api";
import { useAuth } from "./useAuth";
import type { Bookmark } from "./types";
import Login from "./components/Login";
import SearchBar from "./components/SearchBar";
import TagSidebar from "./components/TagSidebar";
import BookmarkGrid from "./components/BookmarkGrid";
import TagEditor from "./components/TagEditor";
import TagManager from "./components/TagManager";

export default function App() {
  const { hasToken, setToken, clear } = useAuth();
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [tags, setTags] = useState<string[]>([]);
  const [activeTag, setActiveTag] = useState("");
  const [query, setQuery] = useState("");
  const [manage, setManage] = useState(false);
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
    const t = setTimeout(reload, 250); // debounce query/tag changes
    return () => clearTimeout(t);
  }, [hasToken, reload]);

  if (!hasToken) return <Login onSubmit={setToken} />;

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white border-b px-6 py-3 flex items-center gap-4">
        <h1 className="text-lg font-semibold">iNav</h1>
        <SearchBar value={query} onChange={setQuery} />
        <button
          onClick={() => setManage((m) => !m)}
          className="ml-auto text-sm text-gray-600 hover:text-gray-900"
        >
          {manage ? "完成" : "管理标签"}
        </button>
        <button onClick={clear} className="text-sm text-gray-500 hover:text-gray-800">
          退出
        </button>
      </header>
      {error && <div className="px-6 py-2 text-red-600">{error}</div>}
      <div className="flex gap-6 p-6">
        <TagSidebar tags={tags} active={activeTag} onSelect={setActiveTag} />
        <main className="flex-1">
          {manage && (
            <div className="mb-4">
              <TagManager tags={tags} onChanged={reload} />
            </div>
          )}
          <BookmarkGrid
            bookmarks={bookmarks}
            onTagClick={setActiveTag}
            renderActions={(b) => <TagEditor b={b} onChanged={reload} />}
          />
        </main>
      </div>
    </div>
  );
}
