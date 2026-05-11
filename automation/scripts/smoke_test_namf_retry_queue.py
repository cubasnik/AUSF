"""smoke_test_namf_retry_queue.py — Namf retry queue drain smoke test.

Scenario:
  1. Scale mock-amf to 0 replicas (stop it) so the first Namf notification
     delivery fails and AUSF enqueues the notification for retry.
  2. Initiate a 5G-AKA authentication with a valid notification_uri that
     points at mock-amf.
  3. Confirm authentication so AUSF attempts to deliver the SUCCESS callback
     and fails (mock-amf is down).
  4. Restart mock-amf so it is reachable again.
  5. Poll AUSF logs / mock-amf /notifications until the SUCCESS callback
     arrives, proving the retry queue drained successfully.
"""
from __future__ import annotations

import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import restart_compose_service, wait_for_container_health
from smoke_client import AUSFClient
from smoke_runtime import (
    AUSF_BASE_URL,
    CONTROL_PLANE_BASE_URL,
    MOCK_AMF_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    load_amf_notifications,
    load_control_plane_authentication_context,
    wait_for_health,
)


COMPOSE_ROOT = ROOT.parent
SUPI = "imsi-250010000000001"
SERVING_NETWORK = "5G:mnc001.mcc001.3gppnetwork.org"
NOTIFY_URI = "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify"
# Allow up to 60 s for the retry queue to drain after mock-amf is restored.
DRAIN_TIMEOUT_SECONDS = 60


def _compose(*args: str) -> None:
    subprocess.run(
        ["docker", "compose", *args],
        cwd=COMPOSE_ROOT,
        check=True,
    )


def _stop_amf() -> None:
    """Stop mock-amf without removing its container so it can be restarted."""
    print("==> stopping mock-amf", flush=True)
    _compose("stop", "mock-amf")


def _start_amf() -> None:
    print("==> starting mock-amf", flush=True)
    _compose("start", "mock-amf")
    wait_for_container_health("mock-amf", timeout_seconds=30)


def _wait_for_success_notification(
    auth_ctx_id: str,
    baseline: int,
    timeout_seconds: int = DRAIN_TIMEOUT_SECONDS,
) -> dict:
    """Poll mock-amf /notifications until a SUCCESS callback for auth_ctx_id arrives."""
    deadline = time.time() + timeout_seconds
    while time.time() < deadline:
        try:
            notifications = load_amf_notifications()[baseline:]
        except Exception:
            time.sleep(1)
            continue
        for notification in notifications:
            if notification.get("authCtxId") == auth_ctx_id and notification.get("authResult") == "SUCCESS":
                return notification
        time.sleep(1)
    raise RuntimeError(
        f"timed out waiting for SUCCESS Namf notification for {auth_ctx_id} "
        f"after {timeout_seconds}s — retry queue did not drain"
    )


def main() -> int:
    print(f"nrf-health:           {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health:           {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")
    print(f"control-plane-health: {wait_for_health(f'{CONTROL_PLANE_BASE_URL}/healthz')}")
    print(f"amf-health:           {wait_for_health(f'{MOCK_AMF_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")

    baseline_count = len(load_amf_notifications())
    print(f"baseline-amf-notification-count: {baseline_count}")

    # Initiate authentication before stopping AMF so the challenge succeeds.
    challenge = client.initiate_authentication(
        SUPI,
        SERVING_NETWORK,
        notification_uri=NOTIFY_URI,
    )
    auth_ctx_id = challenge["authCtxId"]
    print(f"challenge: {auth_ctx_id}")

    control_plane_context = load_control_plane_authentication_context(SUPI)
    expected_res_star = control_plane_context["xresStar"]

    # Stop mock-amf so the SUCCESS notification delivery will fail and be queued.
    _stop_amf()

    confirmed = client.confirm_authentication(auth_ctx_id, expected_res_star)
    print(f"confirmed: {confirmed}")
    # AUSF should have attempted delivery and failed; it must not raise an error
    # on the confirm endpoint itself — only the background retry is affected.
    assert confirmed.get("authResult") == "AUTHENTICATION_VERIFIED", (
        f"unexpected confirm result: {confirmed}"
    )

    # Verify that no notification arrived while AMF was down.
    immediate_notifications = load_amf_notifications()[baseline_count:]
    amf_notified_immediately = any(
        n.get("authCtxId") == auth_ctx_id for n in immediate_notifications
    )
    if amf_notified_immediately:
        raise AssertionError(
            "Namf SUCCESS notification arrived while mock-amf was stopped — "
            "retry queue was not exercised"
        )
    print("amf-notification-while-stopped: none (expected)")

    # Restore mock-amf and wait for the retry queue to drain.
    _start_amf()

    notification = _wait_for_success_notification(auth_ctx_id, baseline_count)
    print(f"retry-queue-drained-notification: {notification}")
    assert notification["supi"] == SUPI
    print("retry-queue-drain: SUCCESS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
