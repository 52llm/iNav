import { useState } from "react";
import { api } from "../api";

export default function TagManager({
  tags,
  onChanged,
}: {
  tags: string[];
  onChanged: () => void;
}) {
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
    const src = sources
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
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
          <input
            list="taglist"
            className="border rounded px-2 py-1"
            placeholder="原标签"
            value={oldName}
            onChange={(e) => setOldName(e.target.value)}
          />
          <input
            className="border rounded px-2 py-1"
            placeholder="新名称"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
          />
          <button onClick={rename} className="bg-blue-600 text-white rounded px-3">
            重命名
          </button>
        </div>
      </div>
      <div>
        <h3 className="font-medium mb-2">合并标签</h3>
        <div className="flex gap-2">
          <input
            className="border rounded px-2 py-1 flex-1"
            placeholder="来源（逗号分隔）"
            value={sources}
            onChange={(e) => setSources(e.target.value)}
          />
          <input
            list="taglist"
            className="border rounded px-2 py-1"
            placeholder="合并到"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          />
          <button onClick={merge} className="bg-blue-600 text-white rounded px-3">
            合并
          </button>
        </div>
      </div>
      <datalist id="taglist">
        {tags.map((t) => (
          <option key={t} value={t} />
        ))}
      </datalist>
      {msg && <p className="text-green-600">{msg}</p>}
    </div>
  );
}
