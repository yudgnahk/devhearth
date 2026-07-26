#!/usr/bin/env bash
# Drive a one-shot read-only scan over JSON-RPC and print
# assets/portfolio/fit/recommendations/report.
# Usage: scan-roots.sh <engine-binary> <root-path> [database-path]
set -euo pipefail

engine="${1:?engine binary required}"
root="${2:?scan root required}"
database="${3:-}"

if [[ ! -x "$engine" ]]; then
  echo "engine not executable: $engine" >&2
  exit 1
fi
if [[ ! -d "$root" ]]; then
  echo "scan root is not a directory: $root" >&2
  exit 1
fi

args=()
if [[ -n "$database" ]]; then
  mkdir -p "$(dirname "$database")"
  args+=(-database "$database")
fi

python3 - "$engine" "$root" "${args[@]+"${args[@]}"}" <<'PY'
import json, subprocess, sys, time

engine = sys.argv[1]
root = sys.argv[2]
engine_args = sys.argv[3:]

proc = subprocess.Popen(
    [engine, *engine_args],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    # Inherit stderr so verbose engine logs cannot fill a pipe and deadlock.
    stderr=None,
    text=True,
    bufsize=1,
)

def send(msg):
    proc.stdin.write(json.dumps(msg) + "\n")
    proc.stdin.flush()

def read():
    line = proc.stdout.readline()
    if not line:
        raise SystemExit("engine closed stdout unexpectedly")
    return json.loads(line)

send({
    "jsonrpc": "2.0",
    "id": "1",
    "method": "engine.hello",
    "params": {"protocolVersions": [1], "schemaVersions": [1]},
})
hello = read()
if "error" in hello:
    raise SystemExit(f"hello failed: {hello['error']}")
print(f"engine {hello['result']['engine']} protocol={hello['result']['protocolVersion']} readOnly={hello['result']['readOnly']}")

send({
    "jsonrpc": "2.0",
    "id": "2",
    "method": "scan.start",
    "params": {"roots": [root], "policyId": "default"},
})
started = read()
if "error" in started:
    raise SystemExit(f"scan.start failed: {started['error']}")
scan_id = started["result"]["scanId"]
print(f"scan {scan_id} root={root}")

deadline = time.time() + 300
while time.time() < deadline:
    msg = read()
    if msg.get("method") != "scan.progress":
        continue
    params = msg["params"]
    phase = params.get("phase", "?")
    entries = params.get("entriesVisited", 0)
    assets = params.get("assetsFound", 0)
    complete = params.get("complete", False)
    print(f"  progress phase={phase} entries={entries} assets={assets}")
    if complete:
        break
else:
    raise SystemExit("scan timed out")

for req_id, method in (
    ("3", "assets.list"),
    ("4", "portfolio.list"),
    ("5", "fit.list"),
    ("6", "recommendations.list"),
    ("7", "report.export"),
):
    send({"jsonrpc": "2.0", "id": req_id, "method": method, "params": {"scanId": scan_id}})
    resp = read()
    if "error" in resp:
        raise SystemExit(f"{method} failed: {resp['error']}")
    print(json.dumps({method: resp["result"]}, indent=2, sort_keys=True))

proc.stdin.close()
proc.wait(timeout=10)
if proc.returncode not in (0, None):
    raise SystemExit(f"engine exited with code {proc.returncode}")
PY
