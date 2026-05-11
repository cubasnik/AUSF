"""smoke_test_concurrent_auth.py — Concurrent 5G-AKA authentication smoke test.

Scenario:
  Launch THREAD_COUNT (20) concurrent threads, each authenticating a distinct
  subscriber (imsi-250010000000001 through imsi-250010000000020).

  Every thread:
    1. Calls initiate_authentication to obtain a 5G-AKA challenge and its
       unique authCtxId.
    2. Fetches the expected xresStar from the control-plane for that SUPI.
    3. Calls confirm_authentication with the correct resStar.
    4. Records the authResult returned by AUSF.

  After all threads have joined the test asserts:
    - Every authentication returned authResult == "SUCCESS".
    - No thread's confirmed supi differs from the SUPI it initiated with
      (cross-contamination check: each authCtxId maps to the correct SUPI).
"""
from __future__ import annotations

import sys
import threading
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import (
    AUSF_BASE_URL,
    CONTROL_PLANE_BASE_URL,
    MOCK_AMF_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    load_control_plane_authentication_context,
    wait_for_health,
)

SERVING_NETWORK = "5G:mnc001.mcc001.3gppnetwork.org"
THREAD_COUNT = 20
# imsi-250010000000001 through imsi-250010000000020 — all registered in
# control-plane/data/subscribers.json with authMethod=5G_AKA (001 is the
# original; 002 is EAP_AKA_PRIME but 5G_AKA is requested explicitly here;
# 003–020 added for this test).
SUPIS = [f"imsi-2500100000000{i:02d}" for i in range(1, THREAD_COUNT + 1)]


@dataclass
class ThreadResult:
    supi: str
    auth_ctx_id: str = ""
    confirmed_supi: str = ""
    auth_result: str = ""
    error: Exception | None = None


def _run_auth(supi: str, result: ThreadResult) -> None:
    """Initiate and confirm one 5G-AKA authentication for *supi*."""
    client = AUSFClient(AUSF_BASE_URL)
    try:
        challenge = client.initiate_authentication(
            supi,
            SERVING_NETWORK,
            auth_type="5G_AKA",
        )
        result.auth_ctx_id = challenge["authCtxId"]

        ctx = load_control_plane_authentication_context(supi)
        expected_res_star = ctx["xresStar"]

        confirmed = client.confirm_authentication(result.auth_ctx_id, expected_res_star)
        result.auth_result = confirmed.get("authResult", "")
        result.confirmed_supi = confirmed.get("supi", "")
    except Exception as exc:
        result.error = exc


def main() -> int:
    print(f"nrf-health:           {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health:           {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")
    print(f"control-plane-health: {wait_for_health(f'{CONTROL_PLANE_BASE_URL}/healthz')}")
    print(f"amf-health:           {wait_for_health(f'{MOCK_AMF_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")

    results = [ThreadResult(supi=supi) for supi in SUPIS]
    threads = [
        threading.Thread(
            target=_run_auth,
            args=(result.supi, result),
            daemon=True,
        )
        for result in results
    ]

    print(f"==> launching {THREAD_COUNT} concurrent auth threads", flush=True)
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join(timeout=60)

    failures: list[str] = []
    for result in results:
        if result.error is not None:
            failures.append(f"SUPI={result.supi}: {result.error}")
            continue
        if result.auth_result != "SUCCESS":
            failures.append(
                f"SUPI={result.supi} authCtxId={result.auth_ctx_id}: "
                f"authResult={result.auth_result!r} (expected SUCCESS)"
            )
            continue
        if result.confirmed_supi and result.confirmed_supi != result.supi:
            failures.append(
                f"SUPI={result.supi} authCtxId={result.auth_ctx_id}: "
                f"confirmed supi={result.confirmed_supi!r} "
                "(cross-contamination detected)"
            )
            continue
        print(
            f"  OK  {result.supi}  authCtxId={result.auth_ctx_id}  "
            f"authResult={result.auth_result}",
            flush=True,
        )

    if failures:
        for msg in failures:
            print(f"FAIL: {msg}", file=sys.stderr, flush=True)
        raise AssertionError(
            f"{len(failures)}/{THREAD_COUNT} concurrent auth(s) failed"
        )

    print(f"concurrent-auth: {THREAD_COUNT}/{THREAD_COUNT} SUCCESS", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
