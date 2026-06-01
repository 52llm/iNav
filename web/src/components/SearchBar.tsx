export default function SearchBar({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <input
      className="border rounded px-3 py-2 w-full max-w-md"
      placeholder="搜索标题 / 网址 / 摘要…"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}
