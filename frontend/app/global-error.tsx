"use client";

// Top-level error boundary. This replaces the ROOT layout when the layout itself
// (or a provider) throws, so the LocaleProvider/AuthProvider are NOT available
// here — it must render its own <html>/<body> and cannot use useT(). We show
// both languages statically so the page is never blank.

import { useEffect } from "react";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("global error boundary:", error);
  }, [error]);

  return (
    <html lang="zh-CN">
      <body
        style={{
          margin: 0,
          minHeight: "100vh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "#fafafa",
          color: "#171717",
          fontFamily:
            "ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif",
        }}
      >
        <div
          style={{
            maxWidth: 420,
            padding: 32,
            textAlign: "center",
            border: "1px solid #e5e5e5",
            borderRadius: 12,
            background: "#fff",
          }}
        >
          <h1 style={{ fontSize: 18, fontWeight: 600, margin: 0 }}>
            服务暂时不可用 · Service temporarily unavailable
          </h1>
          <p style={{ marginTop: 12, fontSize: 14, color: "#525252" }}>
            发生了意外错误,请稍后重试。
            <br />
            An unexpected error occurred. Please try again.
          </p>
          {error.digest && (
            <p style={{ marginTop: 8, fontSize: 12, color: "#a3a3a3", fontFamily: "monospace" }}>
              ref: {error.digest}
            </p>
          )}
          <button
            onClick={() => reset()}
            style={{
              marginTop: 20,
              padding: "8px 16px",
              fontSize: 14,
              fontWeight: 500,
              color: "#fff",
              background: "#171717",
              border: "none",
              borderRadius: 6,
              cursor: "pointer",
            }}
          >
            重试 · Try again
          </button>
        </div>
      </body>
    </html>
  );
}
