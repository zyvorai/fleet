#!/usr/bin/env python3
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
"""Judge pass/fail for a scripts/ci/soak.sh run from its summary.json.

Criteria (single-writer soak — not an HA claim):

  - agent_stays_up:     fleet-agent remains alive across every WAN-loss cycle
                        (offline autonomy while control plane is down).
  - sites_recover:      after each restore, at least one site reports online
                        within the post-restore sample window.
  - bounded_growth:     process RSS does not show unbounded growth
                        (mean of the last 10% of samples vs. the first 10%).
  - readiness_bound:    every WAN-loss cycle's time-to-ready stays under the
                        configured ceiling (no cycle timed out).
  - no_event_regression: zyvor_fleet_events_total does not go backwards.

Usage: soak-check.py <path/to/summary.json>
Exits 0 if every non-skipped criterion passes, 1 otherwise.
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

CRITERIA: list[dict] = []


def verdict(name: str, status: str, detail: str) -> None:
    CRITERIA.append({"id": name, "status": status, "detail": detail})
    mark = {"pass": "PASS", "fail": "FAIL", "skip": "SKIP"}[status]
    print(f"[{mark}] {name} — {detail}")


def load_jsonl(path: Path) -> list[dict]:
    if not path.exists():
        return []
    out = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out


def check_agent_stays_up(wan_cycles: list[dict]) -> None:
    if not wan_cycles:
        verdict("agent_stays_up", "skip", "no WAN-loss cycles recorded")
        return
    dead = [
        c.get("cycle")
        for c in wan_cycles
        if not c.get("agent_alive_during_outage", False)
    ]
    if dead:
        verdict("agent_stays_up", "fail", f"agent died during outage cycles={dead}")
    else:
        verdict(
            "agent_stays_up",
            "pass",
            f"{len(wan_cycles)} cycle(s), agent stayed up during every CP outage",
        )


def check_sites_recover(wan_cycles: list[dict]) -> None:
    if not wan_cycles:
        verdict("sites_recover", "skip", "no WAN-loss cycles recorded")
        return
    failures = []
    for c in wan_cycles:
        post = c.get("post_restore_sites") or {}
        online = int(post.get("online") or 0)
        if online < 1:
            failures.append(f"cycle {c.get('cycle')}: online={online}")
    if failures:
        verdict("sites_recover", "fail", "; ".join(failures))
    else:
        verdict(
            "sites_recover",
            "pass",
            f"{len(wan_cycles)} cycle(s), >=1 site online after each restore",
        )


def check_bounded_growth(samples: list[dict]) -> None:
    if len(samples) < 10:
        verdict("bounded_growth", "skip", f"only {len(samples)} samples — too few to judge")
        return

    def series(key: str) -> list[int]:
        return [
            int((s.get(key) or {}).get("mem_bytes", 0) or 0)
            for s in samples
            if int((s.get(key) or {}).get("mem_bytes", 0) or 0) > 0
        ]

    failures = []
    for label, key in (("fleetd", "cp_stats"), ("fleet-agent", "agent_stats")):
        vals = series(key)
        if len(vals) < 10:
            continue
        n = max(1, len(vals) // 10)
        first = sum(vals[:n]) / n
        last = sum(vals[-n:]) / n
        if first <= 0:
            continue
        ratio = last / first
        if ratio > 2.0:
            failures.append(
                f"{label}: mem grew {ratio:.2f}x (first~{first:.0f}B last~{last:.0f}B)"
            )

    if failures:
        verdict("bounded_growth", "fail", "; ".join(failures))
    else:
        verdict("bounded_growth", "pass", "no process showed >2x RSS growth over the run")


def check_readiness_bound(summary: dict, wan_cycles: list[dict]) -> None:
    if not wan_cycles:
        verdict("readiness_bound", "skip", "no WAN-loss cycles recorded")
        return
    ceiling = summary.get("ready_ceiling_s", 30)
    times = [c.get("time_to_ready_s", -1) for c in wan_cycles]
    timeouts = [c["cycle"] for c, t in zip(wan_cycles, times) if t is None or t < 0]
    over = [
        (c["cycle"], t)
        for c, t in zip(wan_cycles, times)
        if t is not None and t >= 0 and t > ceiling
    ]
    if timeouts or over:
        verdict(
            "readiness_bound",
            "fail",
            f"timed out cycles={timeouts} over-ceiling={over} (ceiling={ceiling}s)",
        )
    else:
        good = [t for t in times if t is not None and t >= 0]
        worst = max(good) if good else 0
        verdict("readiness_bound", "pass", f"max time_to_ready={worst}s <= ceiling={ceiling}s")


def check_no_event_regression(summary: dict) -> None:
    m = summary.get("metrics", {})
    start = int(m.get("events_start") or 0)
    end = int(m.get("events_end") or 0)
    if end < start:
        verdict("no_event_regression", "fail", f"events_total {start} -> {end}")
    else:
        verdict("no_event_regression", "pass", f"events_total {start} -> {end}")


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: soak-check.py <path/to/summary.json>", file=sys.stderr)
        return 2

    summary_path = Path(sys.argv[1])
    summary = json.loads(summary_path.read_text())
    outdir = summary_path.parent

    if summary.get("ha_claim") is True:
        print("ERROR: summary claims HA — soak-check refuses to bless that", file=sys.stderr)
        return 1

    wan_cycles = load_jsonl(outdir / summary.get("wan_cycles_path", "wan-cycles.jsonl"))
    samples = load_jsonl(outdir / summary.get("samples_path", "samples.jsonl"))

    check_agent_stays_up(wan_cycles)
    check_sites_recover(wan_cycles)
    check_bounded_growth(samples)
    check_readiness_bound(summary, wan_cycles)
    check_no_event_regression(summary)

    report_path = outdir / "soak-check.json"
    report_path.write_text(json.dumps({"criteria": CRITERIA, "ha_claim": False}, indent=2) + "\n")

    failed = [c for c in CRITERIA if c["status"] == "fail"]
    if failed:
        print(f"\n{len(failed)} criterion(criteria) failed")
        return 1
    print("\nall soak criteria passed (or were skipped for lack of data)")
    print("note: this is a single-writer resilience drill, not production HA")
    return 0


if __name__ == "__main__":
    sys.exit(main())
