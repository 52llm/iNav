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
