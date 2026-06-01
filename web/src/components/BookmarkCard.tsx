import type { ReactNode } from "react";
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
  children?: ReactNode;
}) {
  return (
    <div className="bg-white rounded-lg shadow-sm border p-4 flex flex-col gap-2">
      <a
        href={b.url}
        target="_blank"
        rel="noreferrer"
        className="flex items-center gap-2 font-medium hover:underline"
      >
        {b.faviconUrl && (
          <img
            src={b.faviconUrl}
            alt=""
            className="w-4 h-4"
            onError={(e) => (e.currentTarget.style.display = "none")}
          />
        )}
        <span className="truncate">{b.title || b.url}</span>
      </a>
      {b.summary && <p className="text-sm text-gray-600 line-clamp-2">{b.summary}</p>}
      <div className="flex flex-wrap gap-1">
        {b.tags.map((t) => (
          <button
            key={t}
            onClick={() => onTagClick(t)}
            className="text-xs bg-gray-100 hover:bg-gray-200 rounded px-2 py-0.5"
          >
            {t}
          </button>
        ))}
        <span className={`text-xs rounded px-2 py-0.5 ${statusBadge[b.status]}`}>{b.status}</span>
      </div>
      {children}
    </div>
  );
}
