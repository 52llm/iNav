import { useState } from "react";

export default function Login({ onSubmit }: { onSubmit: (token: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <form
        className="bg-white p-6 rounded-lg shadow w-80 space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (value.trim()) onSubmit(value.trim());
        }}
      >
        <h1 className="text-lg font-semibold">登录 iNav</h1>
        <input
          className="w-full border rounded px-3 py-2"
          type="password"
          placeholder="Token"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          autoFocus
        />
        <button className="w-full bg-blue-600 text-white rounded py-2">进入</button>
      </form>
    </div>
  );
}
