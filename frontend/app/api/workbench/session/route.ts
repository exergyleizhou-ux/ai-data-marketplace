import { NextRequest, NextResponse } from "next/server";

const backend = process.env.BACKEND_API_BASE_URL ?? "http://127.0.0.1:8080/api/v1";

export async function POST(req: NextRequest) {
  const origin = req.headers.get("origin");
  let validOrigin=false; try { validOrigin=!!origin&&new URL(origin).origin===req.nextUrl.origin } catch {}
  const csrf=req.cookies.get("oasis_csrf")?.value;
  if (!validOrigin || !csrf || req.headers.get("x-csrf-token")!==csrf) return NextResponse.json({ error: "forbidden" }, { status: 403 });
  const response = await fetch(`${backend}/workbench/token`, {
    method: "POST", headers: { "content-type": "application/json", cookie: req.headers.get("cookie") ?? "" },
    body: await req.text() || "{}", cache: "no-store",
  });
  const payload = await response.json();
  if (!response.ok || payload.code !== 0) return NextResponse.json({ error: "workbench session unavailable" }, { status: response.status });
  const out = NextResponse.json({ workspace_id: payload.data.workspace_id, expires_in: payload.data.expires_in });
  out.cookies.set("oasis_workbench", payload.data.token, { httpOnly: true, secure: process.env.NODE_ENV === "production", sameSite: "strict", path: "/", maxAge: Math.min(payload.data.expires_in, 300) });
  out.headers.set("cache-control", "no-store");
  return out;
}
