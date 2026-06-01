import { useState, useCallback } from "react";

const TOKEN_KEY = "inav_token";

export function useAuth() {
  const [token, setTokenState] = useState<string>(() => localStorage.getItem(TOKEN_KEY) ?? "");

  const setToken = useCallback((t: string) => {
    localStorage.setItem(TOKEN_KEY, t);
    setTokenState(t);
  }, []);

  const clear = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY);
    setTokenState("");
  }, []);

  return { token, hasToken: token.length > 0, setToken, clear };
}
