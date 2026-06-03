"use client";

// Lightweight, dependency-free i18n for the App Router. Strings are co-located
// at the call site as t(zh, en) — no key files, so there is never a "missing
// key", and surfaces can be localized incrementally without breaking anything
// (an unconverted string simply stays Chinese, the current behavior).
//
// Default is Chinese (国内). On first load, a non-Chinese browser locale flips
// to English (国外); the choice is then persisted. Server and the first client
// render both use zh, so there is no hydration mismatch — the locale switch
// happens in an effect.

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Lang = "zh" | "en";

type Ctx = {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (zh: string, en: string) => string;
};

const LocaleContext = createContext<Ctx>({ lang: "zh", setLang: () => {}, t: (zh) => zh });

const STORAGE_KEY = "vo_lang";

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>("zh");

  useEffect(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved === "en" || saved === "zh") {
        setLangState(saved);
        return;
      }
      if (typeof navigator !== "undefined" && navigator.language && !navigator.language.toLowerCase().startsWith("zh")) {
        setLangState("en");
      }
    } catch {
      /* ignore */
    }
  }, []);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    try {
      localStorage.setItem(STORAGE_KEY, l);
    } catch {
      /* ignore */
    }
  }, []);

  // t is stable across renders and only changes when lang changes, so it is safe
  // to list in hook dependency arrays.
  const t = useCallback((zh: string, en: string) => (lang === "en" ? en : zh), [lang]);

  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t]);
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}

// useT returns the current language, a setter, and t(zh, en).
export function useT() {
  return useContext(LocaleContext);
}

// LangToggle is a compact 中 / EN switch.
export function LangToggle({ className = "" }: { className?: string }) {
  const { lang, setLang } = useT();
  return (
    <button
      type="button"
      onClick={() => setLang(lang === "en" ? "zh" : "en")}
      className={`rounded-md border border-neutral-300 px-2 py-1 text-xs text-neutral-600 hover:bg-neutral-50 ${className}`}
      aria-label={lang === "en" ? "切换到中文" : "Switch to English"}
      title={lang === "en" ? "切换到中文" : "Switch to English"}
    >
      {lang === "en" ? "中文" : "EN"}
    </button>
  );
}
