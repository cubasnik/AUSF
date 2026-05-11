from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import (
    AUSF_BASE_URL,
    MOCK_AMF_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    build_eap_aka_prime_response_payload,
    build_eap_aka_prime_synchronization_failure_payload,
    get_eap_context,
    load_amf_notifications,
    load_control_plane_authentication_context,
    wait_for_health,
)


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
    eap_ctx = get_eap_context(supi, challenge["eapSession"]["payload"])
    refreshed = client.confirm_eap_authentication(
        challenge["authCtxId"],
        build_eap_aka_prime_synchronization_failure_payload("00" * 14, eap_ctx),
    )
    print(f"sync-failure-confirmed: {refreshed}")

    assert refreshed["authCtxId"] == challenge["authCtxId"]
    assert refreshed["authResult"] == "ONGOING"
    assert refreshed["eapSession"]["payload"] != challenge["eapSession"]["payload"]

    refreshed_context = client.get_authentication_context(challenge["authCtxId"])
    assert refreshed_context["status"] == "CHALLENGE_SENT"
    assert refreshed_context["eapSession"]["payload"] == refreshed["eapSession"]["payload"]
    print(f"refreshed-context: {refreshed_context}")

    notifications = load_amf_notifications()[baseline_notification_count:]
    assert not notifications
    print("amf-notifications: []")

    control_plane_context = load_control_plane_authentication_context(supi)
    eap_ctx2 = get_eap_context(supi, refreshed["eapSession"]["payload"])
    eap_payload = build_eap_aka_prime_response_payload(control_plane_context["xresStar"], eap_ctx2)
    final_confirmed = client.confirm_eap_authentication(challenge["authCtxId"], eap_payload)
    print(f"final-confirmed: {final_confirmed}")

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