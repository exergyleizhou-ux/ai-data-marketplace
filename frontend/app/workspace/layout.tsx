"use client";

import { useEffect } from "react";

/** Full-bleed Oasis shell — goal:d6aa846b round9 */
export default function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    document.body.classList.add("workspace-embed");
    return () => document.body.classList.remove("workspace-embed");
  }, []);

  return <div className="workspace-frame h-[calc(100vh-3.5rem)] w-full">{children}</div>;
}