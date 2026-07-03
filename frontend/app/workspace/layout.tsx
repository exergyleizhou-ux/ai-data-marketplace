"use client";

import { useEffect } from "react";

/** Full-bleed Oasis shell: keep Nav, hide footer padding, host Lumen iframes. */
export default function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    document.body.classList.add("workspace-embed");
    return () => document.body.classList.remove("workspace-embed");
  }, []);

  return <div className="workspace-frame h-[calc(100vh-3.5rem)] w-full">{children}</div>;
}