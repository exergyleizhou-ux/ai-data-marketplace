import { NextRequest, NextResponse } from "next/server";

export type ServiceStatus = { status: "ok" | "degraded" | "down"; reason_code: string; next_action: string };
type Probe = { key: string; url?: string; missing?: ServiceStatus; degraded?: boolean };
const down = (reason_code: string, next_action: string): ServiceStatus => ({ status: "down", reason_code, next_action });
const degraded = (reason_code: string, next_action: string): ServiceStatus => ({ status: "degraded", reason_code, next_action });
const ok = (): ServiceStatus => ({ status: "ok", reason_code: "ready", next_action: "none" });

function probes(): Probe[] {
  const backend = (process.env.BACKEND_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1").replace(/\/api\/v1\/?$/, "");
  return [
    { key: "backend", url: `${backend}/readyz` },
    { key: "code", url: `${process.env.LUMEN_SERVE_URL ?? "http://127.0.0.1:8787"}/health` },
    { key: "lab", url: `${process.env.LUMEN_LAB_URL ?? "http://127.0.0.1:18992"}/api/lab/health` },
    { key: "science_bridge", url: `${process.env.LUMEN_SCIENCE_URL ?? "http://127.0.0.1:18990"}/health` },
    { key: "workspace", missing: process.env.WORKBENCH_DATABASE_URL ? ok() : degraded("workspace_persistence_unconfigured", "configure_workbench_database") },
    { key: "provider", missing: process.env.LUMEN_PROVIDER_CONFIGURED === "true" ? ok() : degraded("provider_unconfigured", "configure_model_provider") },
    { key: "storage", missing: process.env.STORAGE_DRIVER || process.env.STORAGE_DIR ? ok() : degraded("storage_unconfigured", "configure_object_storage") },
    { key: "compute", missing: process.env.COMPUTE_RUNNER ? ok() : degraded("compute_unconfigured", "configure_compute_runner") },
  ];
}
async function inspect(probe: Probe): Promise<[string, ServiceStatus]> {
  if (!probe.url) return [probe.key, probe.missing ?? down("configuration_missing", "configure_service")];
  try {
    const response = await fetch(probe.url, { method: "GET", cache: "no-store", signal: AbortSignal.timeout(2500), headers: { Accept: "application/json" } });
    return [probe.key, response.ok ? ok() : down(`${probe.key}_unavailable`, `restart_${probe.key}`)];
  } catch {
    return [probe.key, down(`${probe.key}_unreachable`, `check_${probe.key}_connection`)];
  }
}
export async function GET(request: NextRequest) {
  if (!request.cookies.get("oasis_workbench")?.value) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  const services = Object.fromEntries(await Promise.all(probes().map(inspect)));
  const values = Object.values(services) as ServiceStatus[];
  const status = values.some(v => v.status === "down") ? "down" : values.some(v => v.status === "degraded") ? "degraded" : "ok";
  return NextResponse.json({ status, services }, { headers: { "Cache-Control": "no-store" } });
}
