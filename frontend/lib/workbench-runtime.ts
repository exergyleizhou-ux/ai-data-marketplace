export type WorkbenchProjectRef = { id: string; title: string };
export type WorkbenchRunRef = { id: string; last_seq: number; terminal: boolean };

export type WorkbenchSnapshot = {
  kind: "lumen.workbench.snapshot";
  version: 1;
  surface: "lab";
  project: WorkbenchProjectRef | null;
  run: WorkbenchRunRef | null;
  pending_approvals: number;
};

export const LAB_RUN_STATUSES = [
  "queued",
  "running",
  "waiting_approval",
  "verifying",
  "succeeded",
  "failed",
  "canceled",
  "timed_out",
  "exhausted",
] as const;

export type LabRunStatus = (typeof LAB_RUN_STATUSES)[number];

export type LabRun = {
  id: string;
  profile: string;
  title: string;
  status: LabRunStatus;
  stop_reason: string;
  error: string;
  version: number;
};

export type LabEvent = { seq: number; kind: string; level: string };

export type LabArtifact = {
  path: string;
  name: string;
  size: number;
  mtime: string;
  previewKind: string;
  bucket: string;
};

export type LabRuntimeDetail = {
  run: LabRun | null;
  events: LabEvent[];
  artifacts: LabArtifact[];
};

export type WorkbenchFetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

const ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$/;
const TERMINAL_STATUSES = new Set<LabRunStatus>([
  "succeeded",
  "failed",
  "canceled",
  "timed_out",
  "exhausted",
]);
const STATUS_SET = new Set<string>(LAB_RUN_STATUSES);

export class WorkbenchRuntimeError extends Error {
  readonly status: number;

  constructor(status: number, operation: string) {
    super(`Workbench runtime ${operation} failed (${status})`);
    this.name = "WorkbenchRuntimeError";
    this.status = status;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasOnlyKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const allowed = new Set(keys);
  return Object.keys(value).every((key) => allowed.has(key)) && keys.every((key) => key in value);
}

function parseProject(value: unknown): WorkbenchProjectRef | null | undefined {
  if (value === null) return null;
  if (!isRecord(value) || !hasOnlyKeys(value, ["id", "title"])) return undefined;
  if (typeof value.id !== "string" || !ID_PATTERN.test(value.id)) return undefined;
  if (typeof value.title !== "string" || value.title.length < 1 || value.title.length > 200) return undefined;
  return { id: value.id, title: value.title };
}

function parseRunRef(value: unknown): WorkbenchRunRef | null | undefined {
  if (value === null) return null;
  if (!isRecord(value) || !hasOnlyKeys(value, ["id", "last_seq", "terminal"])) return undefined;
  if (typeof value.id !== "string" || !ID_PATTERN.test(value.id)) return undefined;
  if (!Number.isSafeInteger(value.last_seq) || (value.last_seq as number) < 0) return undefined;
  if (typeof value.terminal !== "boolean") return undefined;
  return { id: value.id, last_seq: value.last_seq as number, terminal: value.terminal };
}

export function parseWorkbenchSnapshot(value: unknown): WorkbenchSnapshot | null {
  if (!isRecord(value)) return null;
  if (!hasOnlyKeys(value, ["kind", "version", "surface", "project", "run", "pending_approvals"])) return null;
  if (value.kind !== "lumen.workbench.snapshot" || value.version !== 1 || value.surface !== "lab") return null;
  const project = parseProject(value.project);
  const run = parseRunRef(value.run);
  if (project === undefined || run === undefined) return null;
  if (!Number.isSafeInteger(value.pending_approvals)) return null;
  const pendingApprovals = value.pending_approvals as number;
  if (pendingApprovals < 0 || pendingApprovals > 1000) return null;
  return {
    kind: "lumen.workbench.snapshot",
    version: 1,
    surface: "lab",
    project,
    run,
    pending_approvals: pendingApprovals,
  };
}

export function isTerminalRunStatus(status: string): status is LabRunStatus {
  return TERMINAL_STATUSES.has(status as LabRunStatus);
}

function requireID(value: string, operation: string): string {
  if (!ID_PATTERN.test(value)) throw new WorkbenchRuntimeError(400, operation);
  return value;
}

async function readJSON(response: Response, operation: string): Promise<unknown> {
  if (!response.ok) throw new WorkbenchRuntimeError(response.status, operation);
  try {
    return await response.json();
  } catch {
    throw new WorkbenchRuntimeError(502, operation);
  }
}

function parseLabRun(value: unknown, expectedID: string): LabRun {
  if (!isRecord(value) || !isRecord(value.run)) throw new WorkbenchRuntimeError(502, "run response");
  const run = value.run;
  if (run.id !== expectedID || typeof run.status !== "string" || !STATUS_SET.has(run.status)) {
    throw new WorkbenchRuntimeError(502, "run response");
  }
  return {
    id: expectedID,
    profile: typeof run.profile === "string" ? run.profile.slice(0, 80) : "",
    title: typeof run.title === "string" ? run.title.slice(0, 240) : "",
    status: run.status as LabRunStatus,
    stop_reason: typeof run.stop_reason === "string" ? run.stop_reason.slice(0, 160) : "",
    error: typeof run.error === "string" ? run.error.slice(0, 1000) : "",
    version: Number.isSafeInteger(run.version) && (run.version as number) >= 0 ? (run.version as number) : 0,
  };
}

function parseEvents(value: unknown): LabEvent[] {
  if (!isRecord(value) || !Array.isArray(value.events)) throw new WorkbenchRuntimeError(502, "events response");
  return value.events
    .slice(-100)
    .filter(isRecord)
    .flatMap((event) => {
      if (!Number.isSafeInteger(event.seq) || (event.seq as number) < 0) return [];
      if (typeof event.kind !== "string" || event.kind.length < 1 || event.kind.length > 80) return [];
      return [{
        seq: event.seq as number,
        kind: event.kind,
        level: typeof event.level === "string" ? event.level.slice(0, 24) : "",
      }];
    });
}

function safeArtifactPath(value: unknown): value is string {
  if (typeof value !== "string" || value.length < 1 || value.length > 1024) return false;
  if (value.startsWith("/") || value.includes("\\") || value.includes("\0")) return false;
  return !value.split("/").some((part) => part === ".." || part === "");
}

function parseArtifacts(value: unknown): LabArtifact[] {
  if (!isRecord(value) || !Array.isArray(value.artifacts)) throw new WorkbenchRuntimeError(502, "artifacts response");
  return value.artifacts
    .slice(0, 100)
    .filter(isRecord)
    .flatMap((artifact) => {
      if (!safeArtifactPath(artifact.path)) return [];
      const fallbackName = artifact.path.split("/").pop() ?? artifact.path;
      return [{
        path: artifact.path,
        name: typeof artifact.name === "string" ? artifact.name.slice(0, 240) : fallbackName,
        size: Number.isSafeInteger(artifact.size) && (artifact.size as number) >= 0 ? (artifact.size as number) : 0,
        mtime: typeof artifact.mtime === "string" ? artifact.mtime.slice(0, 80) : "",
        previewKind: typeof artifact.previewKind === "string" ? artifact.previewKind.slice(0, 40) : "",
        bucket: typeof artifact.bucket === "string" ? artifact.bucket.slice(0, 80) : "",
      }];
    });
}

export async function loadLabRuntime(
  snapshotValue: unknown,
  fetcher: WorkbenchFetcher = fetch,
  signal?: AbortSignal,
): Promise<LabRuntimeDetail> {
  const snapshot = parseWorkbenchSnapshot(snapshotValue);
  if (!snapshot) throw new WorkbenchRuntimeError(400, "snapshot validation");

  const runID = snapshot.run?.id;
  const projectID = snapshot.project?.id;
  const runRequest = runID
    ? fetcher(`/api/lab/runs/${requireID(runID, "run id")}`, { cache: "no-store", signal })
    : null;
  const eventsRequest = runID
    ? fetcher(`/api/lab/runs/${requireID(runID, "run id")}/events?after=0`, { cache: "no-store", signal })
    : null;
  const artifactQuery = projectID ? new URLSearchParams({ project_id: projectID }) : null;
  const artifactsRequest = artifactQuery
    ? fetcher(`/api/lab/artifacts?${artifactQuery.toString()}`, { cache: "no-store", signal })
    : null;

  const [runResponse, eventsResponse, artifactsResponse] = await Promise.all([
    runRequest,
    eventsRequest,
    artifactsRequest,
  ]);

  return {
    run: runResponse && runID ? parseLabRun(await readJSON(runResponse, "run query"), runID) : null,
    events: eventsResponse ? parseEvents(await readJSON(eventsResponse, "events query")) : [],
    artifacts: artifactsResponse ? parseArtifacts(await readJSON(artifactsResponse, "artifacts query")) : [],
  };
}

export async function cancelLabRun(
  runID: string,
  fetcher: WorkbenchFetcher = fetch,
  signal?: AbortSignal,
): Promise<void> {
  const safeRunID = requireID(runID, "cancel run id");
  const response = await fetcher(`/api/lab/runs/${safeRunID}/cancel`, {
    method: "POST",
    headers: { Accept: "application/json" },
    signal,
  });
  const body = await readJSON(response, "cancel");
  if (!isRecord(body) || body.ok !== true || body.run_id !== safeRunID) {
    throw new WorkbenchRuntimeError(502, "cancel response");
  }
}
