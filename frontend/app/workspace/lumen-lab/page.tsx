"use client";

export default function LumenLabWorkspacePage() {
  return (
    <iframe
      src="/lumen-lab/?embed=1"
      title="Lumen Science 实验室"
      className="h-full w-full border-0 bg-paper"
      allow="clipboard-read; clipboard-write"
      sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
    />
  );
}
