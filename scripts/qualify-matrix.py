#!/usr/bin/env python3
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
"""Software qualification matrix for Zyvor Fleet."""
from __future__ import annotations

import json
import os
import pathlib
import subprocess
import sys
from datetime import datetime, timezone

ROOT = pathlib.Path(__file__).resolve().parents[1]
EVIDENCE = ROOT / "evidence" / "qualification"


def run(cmd, **kwargs):
    return subprocess.run(cmd, cwd=ROOT, text=True, capture_output=True, **kwargs)


def row(results, name, status, detail=""):
    results.append({"id": name, "status": status, "detail": detail, "class": "software"})
    mark = "PASS" if status == "pass" else ("SKIP" if status == "skip" else "FAIL")
    print(f"[{mark}] {name}" + (f" — {detail}" if detail else ""))


def main():
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    results = []
    started = datetime.now(timezone.utc).isoformat()

    proc = run(["sh", "-c", 'test -z "$(gofmt -l cmd internal webui)"'])
    row(results, "gofmt", "pass" if proc.returncode == 0 else "fail", (proc.stdout + proc.stderr)[-200:])

    proc = run(["go", "vet", "./..."], timeout=120)
    row(results, "go_vet", "pass" if proc.returncode == 0 else "fail", (proc.stdout + proc.stderr)[-300:])

    proc = run(["go", "test", "-race", "./..."], timeout=300)
    row(results, "unit_race", "pass" if proc.returncode == 0 else "fail", (proc.stdout + proc.stderr)[-400:])

    proc = run(["go", "test", "./internal/server/", "-count=1", "-run", "OTA"], timeout=120)
    row(results, "ota_contract", "pass" if proc.returncode == 0 else "fail", (proc.stdout + proc.stderr)[-300:])

    proc = run(["node", "--check", "webui/static/app.js"], timeout=30)
    if proc.returncode == 0:
        row(results, "web_js_syntax", "pass")
    elif proc.returncode == 127 or "No such file" in (proc.stderr or ""):
        row(results, "web_js_syntax", "skip", "node not installed")
    else:
        row(results, "web_js_syntax", "fail", proc.stderr[-200:])

    proc = run(["make", "build"], timeout=180)
    row(results, "build_binaries", "pass" if proc.returncode == 0 else "fail", (proc.stdout + proc.stderr)[-300:])

    proc = run(["python3", "scripts/live-smoke.py"], timeout=180)
    row(results, "live_smoke", "pass" if proc.returncode == 0 else "fail", (proc.stdout + proc.stderr)[-400:])

    proc = run(["bash", "scripts/restore-drill.sh"], timeout=60)
    row(
        results,
        "backup_restore_drill",
        "pass" if proc.returncode == 0 else "fail",
        (proc.stdout + proc.stderr)[-300:],
    )

    for name, detail in [
        ("backup_restore_live_volume", "operator-signed — real PVC/`--data` stop→restore→start; ops-checklist.md"),
        ("multi_site_wan_loss", "operator-signed lab drill"),
        ("ota_zyvor_otad_integration", "lab or zyvor-ota CI lab-substitute (fleet-ref HTTPS commit)"),
    ]:
        row(results, name, "skip", detail)

    report = {
        "generated_at": started,
        "finished_at": datetime.now(timezone.utc).isoformat(),
        "product": "zyvor-fleet",
        "version": "0.3.0",
        "host": os.uname().sysname if hasattr(os, "uname") else "unknown",
        "results": results,
        "software_pass": all(r["status"] == "pass" for r in results if r["status"] != "skip"),
        "ops_claimed": False,
        "note": "Ops/lab rows are skip until evidence/qualification/ops-checklist.md is signed.",
    }
    out = EVIDENCE / "software-matrix.json"
    out.write_text(json.dumps(report, indent=2) + "\n")
    print(f"\nwrote {out}")
    if not report["software_pass"]:
        sys.exit(1)
    print("software qualification rows passed; ops checklist still required for production")


if __name__ == "__main__":
    main()
