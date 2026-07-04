"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { tokenStore } from "@/lib/api";

const MSG_AUTH = "lumen-science:oasis-auth";
const MSG_REQUEST = "lumen-science:request-oauth";

export default function LumenScienceWorkspacePage() {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [src, setSrc] = useState<string | null>(null);
  const triedLocal = useRef(false);

  // Try local Lumen first via iframe load — no fetch, no CORS, no cert issues.
  // If it loads within 2s, use local (sandbox works). Otherwise use VPS proxy.
  const onLocalLoad = useCallback(() => {
    if (!triedLocal.current) return;
    // Local Lumen responded — keep using it
  }, []);

  const tryLocal = useCallback(() => {
    if (triedLocal.current) return;
    triedLocal.current = true;
    const timeout = setTimeout(() => {
      // Local didn't respond in time — fallback to VPS
      setSrc("/lumen-science/?embed=1&oasis=1");
    }, 2000);
    // Use a hidden test iframe
    const testFrame = document.createElement("iframe");
    testFrame.src = "https://127.0.0.1:18993/?embed=1&oasis=1";
    testFrame.style.display = "none";
    testFrame.onload = () => {
      clearTimeout(timeout);
      setSrc("https://127.0.0.1:18993/?embed=1&oasis=1");
      testFrame.remove();
    };
    testFrame.onerror = () => {
      clearTimeout(timeout);
      setSrc("/lumen-science/?embed=1&oasis=1");
      testFrame.remove();
    };
    document.body.appendChild(testFrame);
  }, []);

  useEffect(() => { tryLocal(); }, [tryLocal]);

  useEffect(() => {
    const sendAuth = () => {
      const token = tokenStore.access;
      iframeRef.current?.contentWindow?.postMessage(
        { type: MSG_AUTH, access_token: token || null },
        "*"
      );
    };

    const onStorage = (e: StorageEvent) => {
      if (e.key === "adm_access" || e.key === "adm_refresh") sendAuth();
    };

    const onMessage = (e: MessageEvent) => {
      if (e.data?.type !== MSG_REQUEST) return;
      if (!tokenStore.access) {
        window.open("/login?next=/workspace/lumen-science", "_blank");
      } else {
        sendAuth();
      }
    };

    sendAuth();
    window.addEventListener("storage", onStorage);
    window.addEventListener("message", onMessage);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener("message", onMessage);
    };
  }, []);

  if (!src) {
    return (
      <div className="flex flex-col items-center justify-center h-full bg-paper gap-4">
        <p className="text-dim text-sm">正在连接 Lumen Science…</p>
        <p className="text-dim text-xs">如长时间未响应，请确认本机已启动 Lumen</p>
      </div>
    );
  }

  return (
    <iframe
      ref={iframeRef}
      src={src}
      title="Lumen Science"
      className="h-full w-full border-0 bg-paper"
      allow="clipboard-read; clipboard-write"
      sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
    />
  );
}
