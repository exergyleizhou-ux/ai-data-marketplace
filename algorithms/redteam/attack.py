#!/usr/bin/env python3
"""Red-team "algorithm" for the §19 adversarial sandbox test suite.

This is NOT a sellable algorithm — it is a deliberately MALICIOUS payload used to
prove the L1 sandbox actually contains a hostile algorithm (design §7, §19). It
must NEVER be registered as an approved/trusted algorithm.

Given a mode (env VO_ATTACK or params._attack), it ATTEMPTS one exfiltration /
abuse technique and records whether it succeeded into /out/result.json. Run
under the platform's hardening flags (see backend .../runner_docker.go
`dockerRunArgs`: --network=none --read-only --cap-drop=ALL --pids-limit
--memory --cpus + RLIMIT/timeout), every attempt must be CONTAINED:

  egress       socket/DNS to the internet      -> must FAIL (--network=none)
  read_host    read /etc/passwd, escape /data   -> must FAIL (read-only + mounts)
  leak_stdout  print fake "raw data" to stdout  -> platform must NOT surface it
  oversize     write > the output cap to /out   -> gate must reject the job
  timeout      sleep past max_runtime_secs      -> runner must kill the job
  oom          allocate huge memory             -> --memory must kill the job

The harness (run-redteam.sh) runs each mode in a real container and asserts
containment. The local test only exercises the introspectable probes (egress /
read_host / leak_stdout) to prove they are GENUINE attempts (they succeed with no
sandbox, so the docker run flipping them to "blocked" is meaningful).
"""
import json
import os
import socket
import sys
import time

MODE = os.environ.get("VO_ATTACK", "")
OUT_DIR = os.environ.get("VO_OUT_DIR", "/out")
DATA_DIR = os.environ.get("VO_DATA_DIR", "/data")
PARAMS_FILE = os.environ.get("VO_PARAMS", "/params.json")


def log(stage, **kw):
    print(json.dumps({"stage": stage, **kw}), flush=True)


def mode():
    if MODE:
        return MODE
    if os.path.exists(PARAMS_FILE):
        try:
            with open(PARAMS_FILE) as f:
                return (json.load(f) or {}).get("_attack", "")
        except (OSError, ValueError):
            return ""
    return ""


def write_result(r):
    try:
        os.makedirs(OUT_DIR, exist_ok=True)
        with open(os.path.join(OUT_DIR, "result.json"), "w") as f:
            json.dump(r, f)
    except OSError as e:
        log("result_write_failed", err=str(e))


def try_egress():
    out = {}
    try:
        socket.setdefaulttimeout(3)
        s = socket.create_connection(("1.1.1.1", 80))
        s.close()
        out["tcp"] = "connected"  # NOT contained
    except OSError as e:
        out["tcp"] = "blocked:" + type(e).__name__
    try:
        socket.getaddrinfo("example.com", 80)
        out["dns"] = "resolved"  # NOT contained
    except OSError as e:
        out["dns"] = "blocked:" + type(e).__name__
    return out


def try_read_host():
    out = {}
    for path in ("/etc/passwd", "/etc/shadow", "/proc/1/environ"):
        try:
            with open(path, "rb") as f:
                out[path] = "read:%d" % len(f.read(64))  # NOT contained
        except OSError as e:
            out[path] = "blocked:" + type(e).__name__
    # Try to escape the read-only rootfs.
    try:
        with open("/escape-probe", "w") as f:
            f.write("x")
        out["rootfs_write"] = "wrote"  # NOT contained (rootfs should be read-only)
    except OSError as e:
        out["rootfs_write"] = "blocked:" + type(e).__name__
    return out


def try_leak_stdout():
    # A real attacker would print raw rows here; the platform must not surface
    # algorithm stdout to the buyer (design §7.4).
    print("LEAK ssn=123-45-6789 name=Alice row=secret", flush=True)
    return {"printed": True}


def try_oversize():
    # Write far more than any sane output cap; the gate must reject the job.
    path = os.path.join(OUT_DIR, "output.bin")
    written = 0
    try:
        os.makedirs(OUT_DIR, exist_ok=True)
        with open(path, "wb") as f:
            chunk = b"A" * (1 << 20)
            for _ in range(256):  # ~256 MiB
                f.write(chunk)
                written += len(chunk)
    except OSError as e:
        return {"written": written, "stopped": type(e).__name__}
    return {"written": written}


def main():
    m = mode()
    log("attack_start", mode=m)
    if m == "egress":
        write_result({"mode": m, "result": try_egress()})
    elif m == "read_host":
        write_result({"mode": m, "result": try_read_host()})
    elif m == "leak_stdout":
        write_result({"mode": m, "result": try_leak_stdout()})
    elif m == "oversize":
        write_result({"mode": m, "result": try_oversize()})
    elif m == "timeout":
        time.sleep(3600)  # runner must kill us
    elif m == "oom":
        # Touch every page so the memory is RESIDENT (a plain bytearray is
        # calloc-lazy and stays on the shared zero page, never hitting the cgroup
        # limit). Under --memory the cgroup OOM killer SIGKILLs us before we
        # finish; if we DO finish, the limit was not enforced.
        blocks = []
        try:
            for _ in range(64):  # 64 x 64 MiB = 4 GiB, all touched
                b = bytearray(64 * 1024 * 1024)
                for i in range(0, len(b), 4096):
                    b[i] = 1
                blocks.append(b)
            write_result({"mode": m, "result": "allocated"})  # NOT contained
        except MemoryError:
            write_result({"mode": m, "result": "MemoryError"})
    else:
        log("unknown_mode", mode=m)
        sys.exit(2)
    log("attack_done", mode=m)


if __name__ == "__main__":
    main()
