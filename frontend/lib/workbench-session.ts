export type WorkbenchSession = { workspace_id: string; expires_in: number };
export async function establishWorkbenchSession(workspaceId?: string): Promise<WorkbenchSession> {
  const res = await fetch("/api/workbench/session", { method: "POST", credentials: "include", headers: { "content-type": "application/json" }, body: JSON.stringify(workspaceId ? { workspace_id: workspaceId } : {}) });
  if (!res.ok) throw new Error("workbench session unavailable");
  return res.json();
}
