"use client";

import { useEffect, useRef, useState, useCallback } from "react";

export default function LumenLabWorkspacePage() {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [src, setSrc] = useState<string | null>(null);
  const triedLocal = useRef(false);

  const tryLocal = useCallback(() => {
    if (triedLocal.current) return;
    triedLocal.current = true;
    const timeout = setTimeout(() => {
      setSrc("/lumen-science/?embed=1&oasis=1"); // show message
    }, 2500);
    const testFrame = document.createElement("iframe");
    testFrame.src = "https://127.0.0.1:18995/?embed=1";
    testFrame.style.display = "none";
    testFrame.onload = () => {
      clearTimeout(timeout);
      setSrc("https://127.0.0.1:18995/?embed=1");
      testFrame.remove();
    };
    testFrame.onerror = () => {
      clearTimeout(timeout);
      // Try HTTP fallback
      const httpFrame = document.createElement("iframe");
      httpFrame.src = "http://127.0.0.1:18992/?embed=1";
      httpFrame.style.display = "none";
      httpFrame.onload = () => {
        setSrc("http://127.0.0.1:18992/?embed=1");
        httpFrame.remove();
      };
      httpFrame.onerror = () => {
        setSrc("/lumen-science/?embed=1&oasis=1");
        httpFrame.remove();
      };
      document.body.appendChild(httpFrame);
      testFrame.remove();
    };
    document.body.appendChild(testFrame);
  }, []);

  useEffect(() => { tryLocal(); }, [tryLocal]);

  if (!src) {
    return (
      <div className="flex flex-col items-center justify-center h-full bg-paper gap-4">
        <p className="text-dim text-sm">正在连接 Lumen 实验室…</p>
      </div>
    );
  }

  if (false) {
    return (
      <div className="flex flex-col items-center justify-center h-full bg-paper gap-4 p-8 text-center">
        <h3 className="text-lg font-semibold">Lumen Science 实验室</h3>
        <p className="text-dim text-sm max-w-md">
          实验室需要本机运行 Lumen。<br/>
          请先在终端执行 <code className="bg-line px-2 py-0.5 rounded text-xs">lumen science lab</code>，
          然后刷新此页面。
        </p>
        <a
          href="http://127.0.0.1:18992/"
          target="_blank"
          rel="noopener"
          className="btn primary text-sm mt-4"
        >
          直接打开实验室 →
        </a>
      </div>
    );
  }

  return (
    <iframe
      ref={iframeRef}
      src={src}
      title="Lumen Science 实验室"
      className="h-full w-full border-0 bg-paper"
      allow="clipboard-read; clipboard-write"
      sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
    />
  );
}
