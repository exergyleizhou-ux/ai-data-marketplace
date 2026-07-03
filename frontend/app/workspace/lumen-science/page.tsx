"use client";

import { useEffect, useRef } from "react";
import { tokenStore } from "@/lib/api";

const MSG_AUTH = "lumen-science:oasis-auth";
const MSG_REQUEST = "lumen-science:request-oauth";

export default function LumenScienceWorkspacePage() {
  const iframeRef = useRef<HTMLIFrameElement>(null);

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

  return (
    <iframe
      ref={iframeRef}
      src="/lumen-science/?embed=1&oasis=1"
      title="Lumen Science"
      className="h-full w-full border-0 bg-paper"
      allow="clipboard-read; clipboard-write"
    />
  );
}