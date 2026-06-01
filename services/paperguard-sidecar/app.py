"""FastAPI wrapper exposing the PaperGuard authenticity screener.

Contract (matches the Go client in backend/internal/modules/dataset/sidecar.go):
  GET  /healthz        -> {"status":"ok","paperguard_version":"..."}
  POST /v1/screen      body = raw CSV/TSV bytes, Content-Type: text/csv
                       -> 200 sidecar result JSON
                       -> 500 {"error": ...}  (Go worker falls back to baseline)

The service is stateless and must run behind the internal network only. Errors
return 500 on purpose so the marketplace worker degrades to its in-process Go
authenticity baseline rather than blocking an upload.
"""
from __future__ import annotations

import tempfile
from pathlib import Path

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from screening import paperguard_version, screen_file

app = FastAPI(title="Verdant Oasis · PaperGuard authenticity sidecar", version="1.0")


@app.get("/healthz")
def healthz() -> dict:
    return {"status": "ok", "paperguard_version": paperguard_version()}


@app.post("/v1/screen")
async def screen(request: Request):
    content = await request.body()
    if not content:
        return JSONResponse(status_code=400, content={"error": "empty body"})
    ct = request.headers.get("content-type", "")
    if "parquet" in ct:
        suffix = ".parquet"
    elif "tsv" in ct or "tab-separated" in ct:
        suffix = ".tsv"
    else:
        suffix = ".csv"
    try:
        with tempfile.NamedTemporaryFile(suffix=suffix) as tmp:
            tmp.write(content)
            tmp.flush()
            return screen_file(Path(tmp.name), suffix)
    except Exception as exc:  # noqa: BLE001 — surface as 500 so the caller falls back
        return JSONResponse(status_code=500, content={"error": str(exc)})
