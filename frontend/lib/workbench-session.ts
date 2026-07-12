export type WorkbenchSession = { workspace_id: string; expires_in: number };
export async function establishWorkbenchSession(workspaceId?: string): Promise<WorkbenchSession> {
  const csrf = typeof document === "undefined" ? "" : document.cookie.split("; ").find(value => value.startsWith("oasis_csrf="))?.slice("oasis_csrf=".length) ?? "";
  const res = await fetch("/api/workbench/session", { method: "POST", credentials: "include", headers: { "content-type": "application/json", "x-csrf-token": decodeURIComponent(csrf) }, body: JSON.stringify(workspaceId ? { workspace_id: workspaceId } : {}) });
  if (!res.ok) throw new Error("workbench session unavailable");
  return res.json();
}
export async function revokeWorkbenchSession(): Promise<void> {
  const csrf = typeof document === "undefined" ? "" : document.cookie.split("; ").find(value => value.startsWith("oasis_csrf="))?.slice("oasis_csrf=".length) ?? "";
  const res = await fetch("/api/workbench/session", { method: "DELETE", credentials: "include", headers: { "x-csrf-token": decodeURIComponent(csrf) } });
  if (!res.ok) throw new Error("workbench session logout failed");
}
