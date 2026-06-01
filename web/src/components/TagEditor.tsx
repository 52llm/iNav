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
        <button
          key={t}
          disabled={busy}
          onClick={() => run(() => api.patchTags(b.id, [], [t]))}
          className="bg-gray-100 rounded px-1 hover:bg-red-100"
          title="移除"
        >
          {t} ✕
        </button>
      ))}
      <button
        disabled={busy}
        onClick={() => run(() => api.retag(b.id))}
        className="text-blue-600 hover:underline"
      >
        重打标
      </button>
      <button
        disabled={busy}
        onClick={() => {
          if (confirm("删除这条收藏？")) run(() => api.deleteBookmark(b.id));
        }}
        className="text-red-600 hover:underline"
      >
        删除
      </button>
    </div>
  );
}
