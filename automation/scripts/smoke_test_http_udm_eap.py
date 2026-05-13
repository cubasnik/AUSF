from __future__ import annotations

import json
import sys

from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import AUSF_BASE_URL, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, build_eap_aka_prime_response_payload, get_eap_context, load_amf_notifications, wait_for_health


def main() -> int:
    supi = "imsi-250010000000002"
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
        auth_type="EAP_AKA_PRIME",
        notification_uri="http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
    )
    assert challenge["authType"] == "EAP_AKA_PRIME"
    eap_ctx = get_eap_context(supi, challenge["eapSession"]["payload"])
    eap_payload = build_eap_aka_prime_response_payload(eap_ctx["xres_star"], eap_ctx)

    confirmed = client.confirm_eap_authentication(challenge["authCtxId"], eap_payload)
    print(f"confirmed: {confirmed}")

    notifications = load_amf_notifications()[baseline_notification_count:]
    matching_notification = next(notification for notification in notifications if notification["authCtxId"] == challenge["authCtxId"])
    assert matching_notification["authResult"] == "SUCCESS"
    assert matching_notification["supi"] == supi
    assert matching_notification["authType"] == "EAP_AKA_PRIME"
    assert matching_notification["_requestPath"] == f"/namf-comm/v1/ue-authentications/{challenge['authCtxId']}/status-notify"
    print(f"amf-notification: {matching_notification}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())