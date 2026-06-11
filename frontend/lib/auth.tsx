"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { api, tokenStore, type LoginResult, type Tokens, type User } from "./api";

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
  // setSession is the post-auth side-door: any flow that authenticates outside
  // the standard login() path (2FA verify, future SSO) must call it so the
  // nav re-renders synchronously instead of waiting for a page reload.
  setSession: (user: User, tokens: Tokens) => void;
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
    if (res.tokens) {
      tokenStore.set(res.tokens);
      if (res.user) setUser(res.user);
    }
    return res;
  }, []);

  const register = useCallback(
    async (account: string, accountType: string, password: string, agreements?: { doc: string; version: string }[]) => {
      const res = await api.register(account, accountType, password, agreements);
      tokenStore.set(res.tokens);
      setUser(res.user);
    },
    [],
  );

  const setSession = useCallback((u: User, tokens: Tokens) => {
    tokenStore.set(tokens);
    setUser(u);
  }, []);

  const logout = useCallback(() => {
    tokenStore.clear();
    setUser(null);
  }, []);

  return (
    <Ctx.Provider value={{ user, loading, login, register, setSession, logout, refresh }}>
      {children}
    </Ctx.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
