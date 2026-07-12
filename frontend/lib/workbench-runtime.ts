export const RUN_STATUSES = ["queued", "running", "waiting_approval", "verifying", "succeeded", "failed", "canceled", "timed_out", "exhausted"] as const;
export type RunStatus = (typeof RUN_STATUSES)[number];
export type VerificationStatus = "idle" | "running" | "passed" | "failed" | "not_run";
export type WorkbenchProjectRef = { id: string; title: string };
export type WorkbenchRunRef = { id: string; last_seq: number; terminal: boolean; status?: RunStatus };
export type WorkbenchSnapshot = {
  kind: "lumen.workbench.snapshot";
  version: 1 | 2;
  surface: "code" | "lab";
  workspace?: { id: string };
  project: WorkbenchProjectRef | null;
  run: WorkbenchRunRef | null;
  pending_approvals: number;
  verification?: VerificationStatus;
  artifact_count?: number;
};
export type LabRunStatus = RunStatus;
export const LAB_RUN_STATUSES = RUN_STATUSES;
export type LabRun = { id: string; profile: string; title: string; status: RunStatus; stop_reason: string; error: string; version: number };
export type LabEvent = { seq: number; kind: string; level: string };
export type LabArtifact = { id?: string; path: string; name: string; size: number; mtime: string; previewKind: string; bucket: string; mime?: string; sha256?: string };
export type WorkbenchApproval = { id: string; run_id: string; step_id: string; tool_call_id: string; risk_level: string; effects: Record<string, boolean>; estimated_cost_micros: number; created_at: string; expires_at: string; decision: string | null; execution_state: string };
export type LabRuntimeDetail = { run: LabRun | null; events: LabEvent[]; artifacts: LabArtifact[]; approvals?: WorkbenchApproval[] };
export type WorkbenchFetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

const ID = /^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$/;
const STATUS = new Set<string>(RUN_STATUSES);
const VERIFY = new Set<string>(["idle", "running", "passed", "failed", "not_run"]);
const TERMINAL = new Set<RunStatus>(["succeeded", "failed", "canceled", "timed_out", "exhausted"]);
const record = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const only = (v: Record<string, unknown>, required: readonly string[], optional: readonly string[] = []) => {
  const allowed = new Set([...required, ...optional]);
  return required.every((key) => key in v) && Object.keys(v).every((key) => allowed.has(key));
};

export class WorkbenchRuntimeError extends Error {
  constructor(readonly status: number, operation: string) { super(`Workbench runtime ${operation} failed (${status})`); this.name = "WorkbenchRuntimeError"; }
}
function parseProject(v: unknown) {
  if (v === null) return null;
  if (!record(v) || !only(v, ["id", "title"]) || typeof v.id !== "string" || !ID.test(v.id) || typeof v.title !== "string" || v.title.length < 1 || v.title.length > 200) return undefined;
  return { id: v.id, title: v.title };
}
function parseWorkspace(v: unknown) {
  if (!record(v) || !only(v, ["id"]) || typeof v.id !== "string" || !ID.test(v.id)) return undefined;
  return { id: v.id };
}
function parseRun(v: unknown, v2: boolean): WorkbenchRunRef | null | undefined {
  if (v === null) return null;
  const keys = v2 ? ["id", "last_seq", "status", "terminal"] : ["id", "last_seq", "terminal"];
  if (!record(v) || !only(v, keys) || typeof v.id !== "string" || !ID.test(v.id) || !Number.isSafeInteger(v.last_seq) || Number(v.last_seq) < 0 || typeof v.terminal !== "boolean") return undefined;
  if (v2 && (typeof v.status !== "string" || !STATUS.has(v.status))) return undefined;
  return { id: v.id, last_seq: Number(v.last_seq), terminal: v.terminal, ...(v2 ? { status: v.status as RunStatus } : {}) };
}
export function parseWorkbenchSnapshot(value: unknown): WorkbenchSnapshot | null {
  if (!record(value) || value.kind !== "lumen.workbench.snapshot") return null;
  const v2 = value.version === 2;
  const required = v2
    ? ["kind", "version", "surface", "workspace", "project", "run", "pending_approvals", "verification", "artifact_count"]
    : ["kind", "version", "surface", "project", "run", "pending_approvals"];
  if (!only(value, required) || (!v2 && value.version !== 1) || (!v2 && value.surface !== "lab") || (v2 && value.surface !== "lab" && value.surface !== "code")) return null;
  const project = parseProject(value.project); const run = parseRun(value.run, v2);
  if (project === undefined || run === undefined || !Number.isSafeInteger(value.pending_approvals) || Number(value.pending_approvals) < 0 || Number(value.pending_approvals) > 1000) return null;
  if (!v2) return { kind: "lumen.workbench.snapshot", version: 1, surface: "lab", project, run, pending_approvals: Number(value.pending_approvals) };
  const workspace = parseWorkspace(value.workspace);
  if (!workspace || typeof value.verification !== "string" || !VERIFY.has(value.verification) || !Number.isSafeInteger(value.artifact_count) || Number(value.artifact_count) < 0 || Number(value.artifact_count) > 100000) return null;
  return { kind: "lumen.workbench.snapshot", version: 2, surface: value.surface as "code" | "lab", workspace, project, run, pending_approvals: Number(value.pending_approvals), verification: value.verification as VerificationStatus, artifact_count: Number(value.artifact_count) };
}
export function parseTrustedWorkbenchMessage(event: { origin: string; source: MessageEventSource | null; data: unknown }, expectedOrigin: string, expectedSource: MessageEventSource | null) {
  return expectedSource && event.origin === expectedOrigin && event.source === expectedSource ? parseWorkbenchSnapshot(event.data) : null;
}
export function isTerminalRunStatus(status: string): status is RunStatus { return TERMINAL.has(status as RunStatus); }
function requireID(v: string, op: string) { if (!ID.test(v)) throw new WorkbenchRuntimeError(400, op); return v; }
async function json(r: Response, op: string) { if (!r.ok) throw new WorkbenchRuntimeError(r.status, op); try { return await r.json(); } catch { throw new WorkbenchRuntimeError(502, op); } }
function parseRuntimeRun(v: unknown, id: string): LabRun {
  if (!record(v) || !record(v.run) || v.run.id !== id || typeof v.run.status !== "string" || !STATUS.has(v.run.status)) throw new WorkbenchRuntimeError(502, "run response");
  const r = v.run; return { id, profile: typeof r.profile === "string" ? r.profile.slice(0, 80) : "", title: typeof r.title === "string" ? r.title.slice(0, 240) : "", status: r.status as RunStatus, stop_reason: typeof r.stop_reason === "string" ? r.stop_reason.slice(0, 160) : "", error: typeof r.error === "string" ? r.error.slice(0, 1000) : "", version: Number.isSafeInteger(r.version) && Number(r.version) >= 0 ? Number(r.version) : 0 };
}
function parseEvents(v: unknown): LabEvent[] { if (!record(v) || !Array.isArray(v.events)) throw new WorkbenchRuntimeError(502, "events response"); return v.events.slice(-100).filter(record).flatMap(e => Number.isSafeInteger(e.seq) && Number(e.seq) >= 0 && typeof e.kind === "string" && e.kind.length > 0 && e.kind.length <= 80 ? [{ seq: Number(e.seq), kind: e.kind, level: typeof e.level === "string" ? e.level.slice(0, 24) : "" }] : []); }
function safePath(v: unknown): v is string { return typeof v === "string" && v.length > 0 && v.length <= 1024 && !v.startsWith("/") && !v.includes("\\") && !v.includes("\0") && !v.split("/").some(p => !p || p === ".."); }
function parseArtifacts(v: unknown): LabArtifact[] { if (!record(v) || !Array.isArray(v.artifacts)) throw new WorkbenchRuntimeError(502, "artifacts response"); return v.artifacts.slice(0, 100).filter(record).flatMap(a => { const id = typeof a.id === "string" && ID.test(a.id) ? a.id : undefined; const path = safePath(a.path) ? a.path : id; if (!path) return []; return [{ id, path, name: typeof a.name === "string" ? a.name.slice(0, 240) : path.split("/").pop()!, size: Number.isSafeInteger(a.size) && Number(a.size) >= 0 ? Number(a.size) : 0, mtime: typeof a.mtime === "string" ? a.mtime.slice(0, 80) : "", previewKind: typeof a.previewKind === "string" ? a.previewKind.slice(0, 40) : "", bucket: typeof a.bucket === "string" ? a.bucket.slice(0, 80) : "", mime: typeof a.mime === "string" ? a.mime.slice(0, 120) : undefined, sha256: typeof a.sha256 === "string" && /^[a-f0-9]{64}$/.test(a.sha256) ? a.sha256 : undefined }]; }); }
function parseApprovals(v: unknown, runID: string): WorkbenchApproval[] { if (!record(v) || !Array.isArray(v.approvals)) throw new WorkbenchRuntimeError(502, "approvals response"); return v.approvals.slice(0, 100).filter(record).flatMap(a => { if (typeof a.id !== "string" || !ID.test(a.id) || a.run_id !== runID || typeof a.risk_level !== "string" || !record(a.effects)) return []; const effects = Object.fromEntries(Object.entries(a.effects).filter(([, value]) => typeof value === "boolean")) as Record<string, boolean>; return [{ id: a.id, run_id: runID, step_id: typeof a.step_id === "string" ? a.step_id.slice(0, 128) : "", tool_call_id: typeof a.tool_call_id === "string" ? a.tool_call_id.slice(0, 128) : "", risk_level: a.risk_level.slice(0, 32), effects, estimated_cost_micros: Number.isSafeInteger(a.estimated_cost_micros) ? Number(a.estimated_cost_micros) : 0, created_at: typeof a.created_at === "string" ? a.created_at : "", expires_at: typeof a.expires_at === "string" ? a.expires_at : "", decision: typeof a.decision === "string" ? a.decision : null, execution_state: typeof a.execution_state === "string" ? a.execution_state.slice(0, 32) : "" }]; }); }
function base(snapshot: WorkbenchSnapshot) { return snapshot.version === 1 ? "/api/lab" : snapshot.surface === "lab" ? "/api/lumen/lab/api/lab" : "/api/lumen/code/v1"; }
export async function loadLabRuntime(value: unknown, fetcher: WorkbenchFetcher = fetch, signal?: AbortSignal): Promise<LabRuntimeDetail> {
  const s = parseWorkbenchSnapshot(value); if (!s) throw new WorkbenchRuntimeError(400, "snapshot validation");
  const id = s.run?.id; const root = base(s); const project = s.project?.id;
  const artifactURL = s.version === 1 && project ? `${root}/artifacts?${new URLSearchParams({ project_id: project })}` : id ? `${root}/runs/${requireID(id, "run id")}/artifacts` : project && s.surface === "lab" ? `${root}/artifacts?${new URLSearchParams({ project_id: project })}` : null;
  const [rr, er, ar, apr] = await Promise.all([id ? fetcher(`${root}/runs/${id}`, { cache: "no-store", signal }) : null, id ? fetcher(`${root}/runs/${id}/events?after=0`, { cache: "no-store", signal }) : null, artifactURL ? fetcher(artifactURL, { cache: "no-store", signal }) : null, id && s.version === 2 ? fetcher(`${root}/runs/${id}/approvals`, { cache: "no-store", signal }) : null]);
  return { run: rr && id ? parseRuntimeRun(await json(rr, "run query"), id) : null, events: er ? parseEvents(await json(er, "events query")) : [], artifacts: ar ? parseArtifacts(await json(ar, "artifacts query")) : [], approvals: apr && id ? parseApprovals(await json(apr, "approvals query"), id) : [] };
}
export async function cancelRuntimeRun(snapshot: WorkbenchSnapshot, fetcher: WorkbenchFetcher = fetch, signal?: AbortSignal) { const id = snapshot.run?.id; if (!id) throw new WorkbenchRuntimeError(400, "cancel run id"); const r = await fetcher(`${base(snapshot)}/runs/${requireID(id, "cancel run id")}/cancel`, { method: "POST", headers: { Accept: "application/json" }, signal }); const body = await json(r, "cancel"); if (!record(body) || body.ok !== true || body.run_id !== id) throw new WorkbenchRuntimeError(502, "cancel response"); }
export async function cancelLabRun(id: string, fetcher: WorkbenchFetcher = fetch, signal?: AbortSignal) { return cancelRuntimeRun({ kind: "lumen.workbench.snapshot", version: 1, surface: "lab", project: null, run: { id, last_seq: 0, terminal: false }, pending_approvals: 0 }, fetcher, signal); }
export async function decideRuntimeApproval(snapshot: WorkbenchSnapshot, approvalID: string, allow: boolean, fetcher: WorkbenchFetcher = fetch) { requireID(approvalID, "approval id"); const root = snapshot.surface === "lab" ? "/api/lumen/lab/api/lab" : "/api/lumen/code/v1"; const response = await fetcher(`${root}/approve`, { method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify({ id: approvalID, allow }) }); await json(response, "approval decision"); }
export function runtimeLinks(snapshot: WorkbenchSnapshot) { const id = snapshot.run?.id; if (!id) return null; const root = base(snapshot); return { approvals: `${root}/runs/${id}/approvals`, artifacts: `${root}/runs/${id}/artifacts`, evidence: `${root}/runs/${id}/evidence` }; }
