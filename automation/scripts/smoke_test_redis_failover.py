"""smoke_test_redis_failover.py — Redis Namf retry-queue failover smoke test.

Scenario:
  This test verifies that AUSF does not panic or crash when the Redis
  backing its Namf notification retry-queue becomes unavailable mid-flight.

  1. Bring up the full compose stack with the docker-compose.redis.yml
     overlay so that ausf-go uses the Redis-backed Namf retry queue
     (AUSF_NAMF_QUEUE_BACKEND=redis, AUSF_NAMF_REDIS_URL=redis://redis:6379/0).
  2. Verify all services are healthy.
  3. Stop mock-amf so that synchronous Namf notification delivery will fail.
  4. Stop the Redis container so that the async enqueue path also fails.
  5. Initiate and confirm a 5G-AKA authentication.
     - The confirm endpoint must succeed (authResult SUCCESS) because Namf
       notification failure is non-fatal for the confirm HTTP response.
     - AUSF must log an ERROR about the Redis enqueue failure but NOT crash.
  6. Verify the AUSF health endpoint returns 200 {"status":"ok"} (no crash).
  7. Tear down.
"""
from __future__ import annotations

import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import wait_for_container_health
from smoke_client import AUSFClient
from smoke_runtime import (
    AUSF_BASE_URL,
    CONTROL_PLANE_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    load_control_plane_authentication_context,
    wait_for_health,
)

COMPOSE_ROOT = ROOT.parent
COMPOSE_FILES = [
    "-f", "docker-compose.yml",
    "-f", "docker-compose.redis.yml",
]
SUPI = "imsi-250010000000001"
SERVING_NETWORK = "5G:mnc001.mcc001.3gppnetwork.org"
HEALTH_CONTAINERS = [
    "mock-udm",
    "mock-nrf",
    "mock-amf",
    "ausf-redis",
    "ausf-control-plane",
    "ausf-go",
]


def _compose(*args: str) -> None:
    command = ["docker", "compose", *COMPOSE_FILES, *args]
    print(f"==> {' '.join(command)}", flush=True)
    subprocess.run(command, cwd=COMPOSE_ROOT, check=True)


def main() -> int:
    try:
        # Step 1: start the full stack with Redis overlay.
        _compose("up", "-d", "--build")
        for container in HEALTH_CONTAINERS:
            wait_for_container_health(container, timeout_seconds=90)
        print("all-containers: healthy", flush=True)

        print(f"nrf-health:           {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
        print(f"udm-health:           {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")
        print(f"control-plane-health: {wait_for_health(f'{CONTROL_PLANE_BASE_URL}/healthz')}")

        client = AUSFClient(AUSF_BASE_URL)
        print(f"ausf-health: {client.health()}")

        # Step 3: stop mock-amf so sync Namf notification delivery fails.
        print("==> stopping mock-amf", flush=True)
        _compose("stop", "mock-amf")

        # Step 4: stop Redis so async enqueue also fails.
        print("==> stopping redis", flush=True)
        _compose("stop", "redis")

        # Step 5: initiate + confirm; AUSF must not crash.
        print(
            "==> initiating authentication with mock-amf and Redis both down",
            flush=True,
        )
        challenge = client.initiate_authentication(SUPI, SERVING_NETWORK)
        auth_ctx_id = challenge["authCtxId"]
        print(f"challenge: authCtxId={auth_ctx_id}", flush=True)

        control_plane_ctx = load_control_plane_authentication_context(SUPI)
        expected_res_star = control_plane_ctx["xresStar"]

        confirmed = client.confirm_authentication(auth_ctx_id, expected_res_star)
        print(f"confirmed: {confirmed}", flush=True)
        assert confirmed.get("authResult") == "SUCCESS", (
            f"unexpected confirm result: {confirmed}"
        )
        print("confirm: OK (Namf enqueue failure is non-fatal)", flush=True)

        # Step 6: AUSF must still be alive after the Redis error.
        health = client.health()
        print(f"ausf-health-after-redis-down: {health}", flush=True)
        assert health.get("status") == "ok", (
            f"AUSF is unhealthy after Redis failure: {health}"
        )
        print("ausf-no-crash: VERIFIED", flush=True)

    finally:
        _compose("down", "-v")

    print("redis-failover: SUCCESS", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
