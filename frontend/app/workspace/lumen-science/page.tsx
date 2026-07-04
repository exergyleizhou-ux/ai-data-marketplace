"use client";

import { useEffect, useRef, useState } from "react";
import { tokenStore } from "@/lib/api";

const MSG_AUTH = "lumen-science:oasis-auth";
const MSG_REQUEST = "lumen-science:request-oauth";
const LOCAL_LUMEN = "http://127.0.0.1:18990";
const VPS_LUMEN = "/lumen-science";

export default function LumenScienceWorkspacePage() {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [src, setSrc] = useState<string | null>(null);

  // Probe local Lumen first — sandbox only works on the user's Mac.
  // VPS proxy is Linux, can't run Claude Science sandbox.
  useEffect(() => {
    let cancelled = false;
    async function probe() {
      try {
        const r = await fetch(LOCAL_LUMEN + "/api/health", { signal: AbortSignal.timeout(1500) });
        if (r.ok && !cancelled) { setSrc(LOCAL_LUMEN + "/?embed=1&oasis=1"); return; }
      } catch (_) {}
      if (!cancelled) setSrc(VPS_LUMEN + "/?embed=1&oasis=1");
    }
    probe();
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    const sendAuth = () => {
      const token = tokenStore.access;
      iframeRef.current?.contentWindow?.postMessage(
        { type: MSG_AUTH, access_token: token || null },
        window.location.origin
      );
    };

    sendAuth();

    const onStorage = (e: StorageEvent) => {
      if (e.key === "adm_access" || e.key === "adm_refresh") sendAuth();
    };

    const onMessage = (e: MessageEvent) => {
      if (e.origin !== window.location.origin) return;
      if (e.data?.type !== MSG_REQUEST) return;
      if (!tokenStore.access) {
        window.open("/login?next=/workspace/lumen-science", "_blank");
      } else {
        sendAuth();
      }
    };

    window.addEventListener("storage", onStorage);
    window.addEventListener("message", onMessage);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener("message", onMessage);
    };
  }, []);

  if (!src) {
    return (
      <div className="flex items-center justify-center h-full bg-paper text-dim text-sm">
        检测本地 Lumen 服务…
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
