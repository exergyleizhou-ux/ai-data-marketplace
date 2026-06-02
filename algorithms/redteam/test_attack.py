"""Local test for the red-team payload. With NO sandbox these probes SUCCEED —
which is exactly the point: it proves they are genuine attacks, so when the
docker harness (run-redteam.sh) runs them under --network=none / --read-only /
limits and they flip to 'blocked'/killed, that demonstrates real containment.

Only the safely-introspectable modes run here (egress / read_host / leak_stdout).
oversize / timeout / oom are exercised by the docker harness only.
"""
import json
import os
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))


def _run(mode):
    out = tempfile.mkdtemp(prefix="redteam-test-")
    env = dict(os.environ, VO_ATTACK=mode, VO_OUT_DIR=out, VO_DATA_DIR=out)
    res = subprocess.run([sys.executable, os.path.join(HERE, "attack.py")], env=env,
                         capture_output=True, text=True)
    result = None
    p = os.path.join(out, "result.json")
    if os.path.exists(p):
        with open(p) as f:
            result = json.load(f)
    return res, result


def test_read_host_is_a_genuine_attack():
    # No sandbox here → it really reads /etc/passwd. Under --read-only + mounts
    # the harness must see "blocked:*".
    res, result = _run("read_host")
    assert res.returncode == 0, res.stderr
    assert result["result"]["/etc/passwd"].startswith("read:"), result


def test_leak_stdout_really_prints():
    res, _ = _run("leak_stdout")
    assert "ssn=" in res.stdout  # the platform must NOT forward this to the buyer


def test_egress_probe_actually_attempts():
    res, result = _run("egress")
    assert res.returncode == 0, res.stderr
    # We don't assert connected vs blocked (the local box may also block egress),
    # only that the probe ran and recorded both a TCP and a DNS attempt.
    assert set(result["result"]) == {"tcp", "dns"}, result


if __name__ == "__main__":
    test_read_host_is_a_genuine_attack()
    test_leak_stdout_really_prints()
    test_egress_probe_actually_attempts()
    print("OK: red-team probes are genuine (containment is asserted by run-redteam.sh under docker)")
