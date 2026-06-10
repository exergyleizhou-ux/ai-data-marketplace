"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { api, tokenStore, type User, type LoginResult } from "./api";

type AuthState = {
  user: User | null;
  loading: boolean;
  login: (account: string, password: string) => Promise<LoginResult>;
  register: (
    account: string,
    accountType: string,
    password: string,
    agreements?: { doc: string; version: string }[],
  ) => Promise<void>;
  logout: () => void;
  refresh: () => Promise<void>;
};

const Ctx = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    if (!tokenStore.access) {
      setUser(null);
      setLoading(false);
      return;
    }
    try {
      setUser(await api.me());
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(async (account: string, password: string) => {
    const res = await api.login(account, password);
    tokenStore.set(res.tokens);
    setUser(res.user);
  }, []);

  const register = useCallback(
    async (account: string, accountType: string, password: string, agreements?: { doc: string; version: string }[]) => {
      const res = await api.register(account, accountType, password, agreements);
      tokenStore.set(res.tokens);
      setUser(res.user);
    },
    [],
  );

  const logout = useCallback(() => {
    tokenStore.clear();
    setUser(null);
  }, []);

  return (
    <Ctx.Provider value={{ user, loading, login, register, logout, refresh }}>{children}</Ctx.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
