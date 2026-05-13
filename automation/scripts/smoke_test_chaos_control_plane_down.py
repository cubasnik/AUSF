"""smoke_test_chaos_control_plane_down.py — Circuit-breaker chaos smoke test.

Scenario:
  1. Verify all services are healthy.
  2. Stop ausf-control-plane to induce connection errors.
  3. Send AUSF_CONTROL_PLANE_BREAKER_FAILURES (default 5) auth requests.
     Each returns 502/503 CONTROL_PLANE_UNAVAILABLE while the breaker
     accumulates consecutive failures.
  4. Send one more auth request — the breaker is now open.
     AUSF must return 503 CONTROL_PLANE_UNAVAILABLE with the message
     "control-plane circuit breaker is open".
  5. Restart ausf-control-plane and wait for it to become healthy.
  6. Wait for the breaker open-timeout to elapse
     (>= AUSF_CONTROL_PLANE_BREAKER_TIMEOUT_SECONDS default 10 s).
  7. Send an auth request — breaker auto-resets to closed, request
     passes through, AUSF returns 201 Created with a valid authCtxId.
"""
from __future__ import annotations

import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import wait_for_container_health
from smoke_client import AUSFClient, AUSFError
from smoke_runtime import (
    AUSF_BASE_URL,
    CONTROL_PLANE_BASE_URL,
    MOCK_AMF_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    wait_for_health,
)

COMPOSE_ROOT = ROOT.parent
SUPI = "imsi-250010000000001"
SERVING_NETWORK = "5G:mnc001.mcc001.3gppnetwork.org"

# Match defaults from internal/config/config.go.
DEFAULT_BREAKER_FAILURES = 5
DEFAULT_BREAKER_TIMEOUT_SECONDS = 10
# Extra margin beyond the breaker timeout to guarantee the window has elapsed.
BREAKER_WAIT_MARGIN_SECONDS = 5


def _compose(*args: str) -> None:
    subprocess.run(
        ["docker", "compose", *args],
        cwd=COMPOSE_ROOT,
        check=True,
    )


def _trip_breaker(client: AUSFClient, threshold: int) -> None:
    """Send `threshold` failed auth requests to open the circuit breaker.

    While the control-plane is stopped every initiate call results in a
    connection error, incrementing the breaker's consecutive-failure counter
    by one.  Each call is expected to return 502 or 503.
    """
    for i in range(threshold):
        try:
            client.initiate_authentication(SUPI, SERVING_NETWORK)
        except AUSFError as error:
            if error.status_code not in (502, 503):
                raise AssertionError(
                    f"[trip {i + 1}/{threshold}] unexpected status "
                    f"{error.status_code}: {error.payload}"
                ) from error
            print(
                f"  trip {i + 1}/{threshold}: "
                f"{error.status_code} {error.payload.get('cause')}",
                flush=True,
            )
        else:
            raise AssertionError(
                f"[trip {i + 1}/{threshold}] expected 502/503 with "
                "control-plane down, got success"
            )


def main() -> int:
    print(f"nrf-health:           {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health:           {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")
    print(f"control-plane-health: {wait_for_health(f'{CONTROL_PLANE_BASE_URL}/healthz')}")
    print(f"amf-health:           {wait_for_health(f'{MOCK_AMF_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")

    # Step 2: stop the control-plane to produce connection errors.
    print("==> stopping ausf-control-plane", flush=True)
    _compose("stop", "ausf-control-plane")

    try:
        # Step 3: trip the circuit breaker with consecutive failures.
        print(
            f"==> tripping circuit breaker "
            f"({DEFAULT_BREAKER_FAILURES} consecutive failures)",
            flush=True,
        )
        _trip_breaker(client, DEFAULT_BREAKER_FAILURES)

        # Step 4: the next request must see an open breaker → 503.
        print("==> verifying open circuit breaker (expecting 503)", flush=True)
        try:
            client.initiate_authentication(SUPI, SERVING_NETWORK)
        except AUSFError as error:
            assert error.status_code == 503, (
                f"expected 503 from open circuit breaker, "
                f"got {error.status_code}: {error.payload}"
            )
            assert error.payload.get("cause") == "CONTROL_PLANE_UNAVAILABLE", (
                f"unexpected cause: {error.payload.get('cause')}"
            )
            print(
                f"circuit-breaker-open: "
                f"{error.status_code} {error.payload.get('cause')}",
                flush=True,
            )
        else:
            raise AssertionError(
                "expected 503 from open circuit breaker, got success"
            )

        # Step 5: restore the control-plane.
        print("==> starting ausf-control-plane", flush=True)
        _compose("start", "ausf-control-plane")
        wait_for_container_health("ausf-control-plane", timeout_seconds=90)
        print("control-plane-health: healthy", flush=True)

        # Step 6: wait for the breaker open-timeout to elapse.
        wait_seconds = DEFAULT_BREAKER_TIMEOUT_SECONDS + BREAKER_WAIT_MARGIN_SECONDS
        print(
            f"==> waiting {wait_seconds}s for circuit-breaker timeout",
            flush=True,
        )
        time.sleep(wait_seconds)

        # Step 7: the breaker resets automatically; next request must succeed.
        print("==> verifying circuit breaker closed (expecting success)", flush=True)
        challenge = client.initiate_authentication(SUPI, SERVING_NETWORK)
        auth_ctx_id = challenge.get("authCtxId", "")
        assert auth_ctx_id, f"expected authCtxId in challenge, got: {challenge}"
        print(f"circuit-breaker-closed: authCtxId={auth_ctx_id}", flush=True)

    except BaseException:
        # Ensure the control-plane is running again even on test failure.
        try:
            _compose("start", "ausf-control-plane")
            wait_for_container_health("ausf-control-plane", timeout_seconds=90)
        except Exception:
            pass
        raise

    print("chaos-control-plane-down: SUCCESS", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
