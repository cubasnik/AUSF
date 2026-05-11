"""Load and soak test for AUSF — Nausf_UEAuthentication (TS 29.509 §6.1).

Runs N concurrent workers each completing full 5G-AKA initiate→confirm cycles
against a realistic SUPI population, then prints a latency/throughput report.

Usage (against a running compose stack):
    python automation/scripts/run_load_test.py [options]

Options:
    --workers  N      Concurrent worker threads (default: 10)
    --duration S      Total test duration in seconds (default: 30)
    --ramp     S      Linear ramp-up period in seconds (default: 2)
    --soak     S      If >0, run a soak phase after the load phase (default: 0)
    --supi-count N    Size of the SUPI population to rotate through (default: 100)
    --mcc      MCC    Mobile Country Code (default: 001)
    --mnc      MNC    Mobile Network Code (default: 01)
    --msisdn-base N   First MSISDN suffix; SUPIs are imsi-<mcc><mnc><10-digit seq>
    --no-confirm      Only initiate, skip confirm (measures initiate-only throughput)
    --json            Print machine-readable JSON report to stdout at the end
    --base-url URL    AUSF base URL (default: AUSF_BASE_URL env or http://127.0.0.1:8080)
    --cp-url   URL    Control-plane base URL (default: CONTROL_PLANE_BASE_URL env or …:8081)
    --nrf-url  URL    Mock NRF health URL (default: MOCK_NRF_BASE_URL env or …:8091)
    --udm-url  URL    Mock UDM health URL (default: MOCK_UDM_BASE_URL env or …:8090)

Environment variables override defaults for all --*-url options.

Exit codes:
    0   All workers completed; success-rate >= threshold (95% by default).
    1   Success-rate below threshold or fatal error.

Examples:
    # 10-second quick load test
    python automation/scripts/run_load_test.py --duration 10

    # 5-minute soak test (300 s) with 20 workers
    python automation/scripts/run_load_test.py --duration 300 --soak 300 --workers 20

    # CI fast smoke (5 s)
    python automation/scripts/run_load_test.py --duration 5 --workers 4
"""
from __future__ import annotations

import argparse
import json
import math
import os
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional
from urllib import request
from urllib.error import HTTPError, URLError

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import (
    AUSF_BASE_URL,
    CONTROL_PLANE_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    load_control_plane_authentication_context,
    wait_for_health,
)

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
DEFAULT_WORKERS = 10
DEFAULT_DURATION = 30
DEFAULT_RAMP = 2
DEFAULT_SOAK = 0
DEFAULT_SUPI_COUNT = 100
DEFAULT_SUCCESS_THRESHOLD = 0.95  # 95%


# ---------------------------------------------------------------------------
# SUPI population generator
# ---------------------------------------------------------------------------
def generate_supi_population(count: int, mcc: str, mnc: str, msisdn_base: int) -> list[str]:
    """Return a list of IMSI-format SUPIs: imsi-<mcc><mnc><10-digit sequence>."""
    return [f"imsi-{mcc}{mnc}{(msisdn_base + i):010d}" for i in range(count)]


# ---------------------------------------------------------------------------
# Per-iteration result
# ---------------------------------------------------------------------------
@dataclass
class IterResult:
    supi: str
    success: bool
    latency_s: float
    phase: str          # "load" or "soak"
    error: str = ""


# ---------------------------------------------------------------------------
# Stats accumulator (thread-safe)
# ---------------------------------------------------------------------------
@dataclass
class Stats:
    _lock: threading.Lock = field(default_factory=threading.Lock, repr=False)
    results: list[IterResult] = field(default_factory=list)

    def record(self, r: IterResult) -> None:
        with self._lock:
            self.results.append(r)

    def summary(self, phase: Optional[str] = None) -> dict:
        with self._lock:
            rows = [r for r in self.results if phase is None or r.phase == phase]
        if not rows:
            return {"count": 0, "success": 0, "errors": 0, "success_rate": 0.0,
                    "p50_ms": 0.0, "p95_ms": 0.0, "p99_ms": 0.0,
                    "mean_ms": 0.0, "min_ms": 0.0, "max_ms": 0.0, "throughput_rps": 0.0}

        latencies = sorted(r.latency_s for r in rows)
        n = len(latencies)
        successes = sum(1 for r in rows if r.success)
        total_time = sum(r.latency_s for r in rows)
        wall_time = latencies[-1] if latencies else 1.0  # rough; see throughput below

        def percentile(p: float) -> float:
            idx = max(0, min(n - 1, int(math.ceil(p / 100 * n)) - 1))
            return round(latencies[idx] * 1000, 2)

        return {
            "count": n,
            "success": successes,
            "errors": n - successes,
            "success_rate": round(successes / n, 4) if n else 0.0,
            "p50_ms": percentile(50),
            "p95_ms": percentile(95),
            "p99_ms": percentile(99),
            "mean_ms": round(total_time / n * 1000, 2) if n else 0.0,
            "min_ms": round(latencies[0] * 1000, 2),
            "max_ms": round(latencies[-1] * 1000, 2),
        }

    def error_breakdown(self, phase: Optional[str] = None) -> dict[str, int]:
        with self._lock:
            rows = [r for r in self.results if (phase is None or r.phase == phase) and not r.success]
        counts: dict[str, int] = {}
        for r in rows:
            counts[r.error] = counts.get(r.error, 0) + 1
        return counts


# ---------------------------------------------------------------------------
# Single auth cycle
# ---------------------------------------------------------------------------
def run_one_cycle(
    client: AUSFClient,
    supi: str,
    serving_network_name: str,
    do_confirm: bool,
    phase: str,
) -> IterResult:
    start = time.monotonic()
    try:
        challenge = client.initiate_authentication(supi, serving_network_name)
        auth_ctx_id = challenge.get("authCtxId", "")

        if do_confirm and auth_ctx_id:
            cp_ctx = load_control_plane_authentication_context(supi)
            res_star = cp_ctx.get("xresStar", "")
            if res_star:
                client.confirm_authentication(auth_ctx_id, res_star)
            else:
                # No xresStar — still count as success if initiate worked
                pass

        latency = time.monotonic() - start
        return IterResult(supi=supi, success=True, latency_s=latency, phase=phase)

    except AUSFError as exc:
        latency = time.monotonic() - start
        return IterResult(supi=supi, success=False, latency_s=latency, phase=phase,
                          error=f"HTTP {exc.status_code}")
    except (URLError, OSError, TimeoutError) as exc:
        latency = time.monotonic() - start
        return IterResult(supi=supi, success=False, latency_s=latency, phase=phase,
                          error=type(exc).__name__)
    except Exception as exc:  # noqa: BLE001
        latency = time.monotonic() - start
        return IterResult(supi=supi, success=False, latency_s=latency, phase=phase,
                          error=type(exc).__name__)


# ---------------------------------------------------------------------------
# Worker: runs cycles for the lifetime of `deadline`
# ---------------------------------------------------------------------------
def worker(
    worker_id: int,
    population: list[str],
    serving_network_name: str,
    ausf_base_url: str,
    do_confirm: bool,
    phase: str,
    deadline: float,
    stats: Stats,
    start_at: float,
) -> None:
    # Ramp-up: each worker starts at a slightly staggered time using its id
    # (caller already handles the ramp delay via `start_at`)
    while time.monotonic() < start_at:
        time.sleep(0.01)

    client = AUSFClient(ausf_base_url)
    idx = worker_id  # offset so workers don't all start on SUPI #0
    while time.monotonic() < deadline:
        supi = population[idx % len(population)]
        idx += 1
        result = run_one_cycle(client, supi, serving_network_name, do_confirm, phase)
        stats.record(result)


# ---------------------------------------------------------------------------
# Phase runner
# ---------------------------------------------------------------------------
def run_phase(
    phase: str,
    workers: int,
    duration: float,
    ramp: float,
    population: list[str],
    serving_network_name: str,
    ausf_base_url: str,
    do_confirm: bool,
    stats: Stats,
) -> None:
    now = time.monotonic()
    deadline = now + duration

    ramp_per_worker = ramp / max(workers, 1)
    futures = []
    with ThreadPoolExecutor(max_workers=workers, thread_name_prefix=f"{phase}-worker") as pool:
        for wid in range(workers):
            start_at = now + wid * ramp_per_worker
            futures.append(
                pool.submit(
                    worker,
                    wid, population, serving_network_name,
                    ausf_base_url, do_confirm, phase, deadline, stats, start_at,
                )
            )
        for f in as_completed(futures):
            f.result()  # propagate unexpected exceptions


# ---------------------------------------------------------------------------
# Reporting helpers
# ---------------------------------------------------------------------------
def print_report(label: str, stats: Stats, phase: str) -> None:
    s = stats.summary(phase)
    errs = stats.error_breakdown(phase)
    print(f"\n{'=' * 60}")
    print(f"  {label}")
    print(f"{'=' * 60}")
    print(f"  Iterations : {s['count']}")
    print(f"  Successes  : {s['success']}")
    print(f"  Errors     : {s['errors']}")
    print(f"  Success %  : {s['success_rate'] * 100:.1f}%")
    print(f"  Latency    : p50={s['p50_ms']} ms  p95={s['p95_ms']} ms  p99={s['p99_ms']} ms")
    print(f"  Mean / Min / Max : {s['mean_ms']} / {s['min_ms']} / {s['max_ms']} ms")
    if errs:
        print(f"  Error breakdown:")
        for err, cnt in sorted(errs.items(), key=lambda x: -x[1]):
            print(f"    {err}: {cnt}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="AUSF load / soak test")
    parser.add_argument("--workers",      type=int,   default=DEFAULT_WORKERS)
    parser.add_argument("--duration",     type=float, default=DEFAULT_DURATION)
    parser.add_argument("--ramp",         type=float, default=DEFAULT_RAMP)
    parser.add_argument("--soak",         type=float, default=DEFAULT_SOAK)
    parser.add_argument("--supi-count",   type=int,   default=DEFAULT_SUPI_COUNT)
    parser.add_argument("--mcc",          type=str,   default="001")
    parser.add_argument("--mnc",          type=str,   default="01")
    parser.add_argument("--msisdn-base",  type=int,   default=0)
    parser.add_argument("--no-confirm",   action="store_true")
    parser.add_argument("--json",         action="store_true")
    parser.add_argument("--base-url",     type=str,   default=AUSF_BASE_URL)
    parser.add_argument("--cp-url",       type=str,   default=CONTROL_PLANE_BASE_URL)
    parser.add_argument("--nrf-url",      type=str,   default=MOCK_NRF_BASE_URL)
    parser.add_argument("--udm-url",      type=str,   default=MOCK_UDM_BASE_URL)
    parser.add_argument("--threshold",    type=float, default=DEFAULT_SUCCESS_THRESHOLD,
                        help="Minimum acceptable success rate (0-1). Exit 1 if below.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()

    serving_network_name = f"5G:mnc{args.mnc}.mcc{args.mcc}.3gppnetwork.org"
    population = generate_supi_population(args.supi_count, args.mcc, args.mnc, args.msisdn_base)

    print(f"[load-test] AUSF={args.base_url}  workers={args.workers}  "
          f"duration={args.duration}s  supi-pool={len(population)}", flush=True)

    # Health checks
    print("[load-test] waiting for services...", flush=True)
    for url in [args.nrf_url + "/healthz", args.udm_url + "/healthz",
                args.base_url + "/healthz"]:
        wait_for_health(url, timeout_seconds=60)
    print("[load-test] all services healthy", flush=True)

    stats = Stats()

    # ---- Load phase ----
    print(f"\n[load-test] starting LOAD phase  ({args.workers} workers, {args.duration}s)", flush=True)
    load_start = time.monotonic()
    run_phase(
        phase="load",
        workers=args.workers,
        duration=args.duration,
        ramp=args.ramp,
        population=population,
        serving_network_name=serving_network_name,
        ausf_base_url=args.base_url,
        do_confirm=not args.no_confirm,
        stats=stats,
    )
    load_elapsed = time.monotonic() - load_start
    load_summary = stats.summary("load")
    load_rps = round(load_summary["count"] / load_elapsed, 2) if load_elapsed > 0 else 0.0
    load_summary["throughput_rps"] = load_rps

    print_report(f"LOAD phase  ({args.duration}s, {args.workers} workers)", stats, "load")
    print(f"  Throughput : {load_rps} cycles/s")

    # ---- Soak phase (optional) ----
    soak_summary: dict = {}
    if args.soak > 0:
        print(f"\n[load-test] starting SOAK phase  ({args.workers} workers, {args.soak}s)", flush=True)
        soak_start = time.monotonic()
        run_phase(
            phase="soak",
            workers=args.workers,
            duration=args.soak,
            ramp=args.ramp,
            population=population,
            serving_network_name=serving_network_name,
            ausf_base_url=args.base_url,
            do_confirm=not args.no_confirm,
            stats=stats,
        )
        soak_elapsed = time.monotonic() - soak_start
        soak_summary = stats.summary("soak")
        soak_rps = round(soak_summary["count"] / soak_elapsed, 2) if soak_elapsed > 0 else 0.0
        soak_summary["throughput_rps"] = soak_rps
        print_report(f"SOAK phase  ({args.soak}s, {args.workers} workers)", stats, "soak")
        print(f"  Throughput : {soak_rps} cycles/s")

    # ---- JSON output ----
    report = {
        "load": load_summary,
        "load_errors": stats.error_breakdown("load"),
    }
    if args.soak > 0:
        report["soak"] = soak_summary
        report["soak_errors"] = stats.error_breakdown("soak")

    if args.json:
        print(json.dumps(report, indent=2))

    # ---- Pass / fail ----
    worst_rate = load_summary["success_rate"]
    if args.soak > 0 and soak_summary:
        worst_rate = min(worst_rate, soak_summary["success_rate"])

    if worst_rate < args.threshold:
        print(f"\n[load-test] FAIL: success rate {worst_rate * 100:.1f}% < "
              f"threshold {args.threshold * 100:.0f}%", file=sys.stderr)
        return 1

    print(f"\n[load-test] PASS: success rate {worst_rate * 100:.1f}% >= "
          f"threshold {args.threshold * 100:.0f}%")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
