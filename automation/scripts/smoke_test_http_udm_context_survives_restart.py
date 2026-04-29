from __future__ import annotations

import hashlib
import json
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import restart_compose_service, wait_for_container_health
from smoke_client import AUSFClient
from smoke_runtime import AUSF_BASE_URL, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, load_amf_notifications, wait_for_health


def load_permanent_key(supi: str) -> str:
    subscribers_path = ROOT.parent / "control-plane" / "data" / "subscribers.json"
    subscribers = json.loads(subscribers_path.read_text(encoding="utf-8"))
    for subscriber in subscribers:
        if subscriber["supi"] == supi:
            return subscriber["permanentKey"]
    raise ValueError(f"seed subscriber not found for {supi}")


def main() -> int:
    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

    print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")
    print(f"amf-health: {wait_for_health(f'{MOCK_AMF_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")
    baseline_notification_count = len(load_amf_notifications())

    challenge = client.initiate_authentication(
        supi,
        serving_network_name,
        notification_uri="http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
    )

    restart_compose_service("ausf-go")
    wait_for_container_health("ausf-go")

    context = client.get_authentication_context(challenge["authCtxId"])
    assert context["authCtxId"] == challenge["authCtxId"]
    assert context["status"] == "CHALLENGE_SENT"

    auth_data = challenge["5gAuthData"]
    permanent_key = load_permanent_key(supi)
    expected_res_star = hashlib.sha256(f"{auth_data['rand']}{auth_data['autn']}{permanent_key}".encode("utf-8")).hexdigest()[:32]
    confirmed = client.confirm_authentication(challenge["authCtxId"], expected_res_star)
    print(f"confirmed-after-restart: {confirmed}")

    notifications = load_amf_notifications()[baseline_notification_count:]
    matching_notification = next(notification for notification in notifications if notification["authCtxId"] == challenge["authCtxId"])
    assert matching_notification["authResult"] == "SUCCESS"
    print(f"amf-notification: {matching_notification}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())