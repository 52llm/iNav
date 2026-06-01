import type { ReactNode } from "react";
import type { Bookmark } from "../types";
import BookmarkCard from "./BookmarkCard";

export default function BookmarkGrid({
  bookmarks,
  onTagClick,
  renderActions,
}: {
  bookmarks: Bookmark[];
  onTagClick: (tag: string) => void;
  renderActions: (b: Bookmark) => ReactNode;
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
