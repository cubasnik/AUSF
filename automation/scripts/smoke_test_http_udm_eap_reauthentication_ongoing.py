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
    build_eap_aka_prime_reauthentication_payload,
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
    assert challenge["authType"] == "EAP_AKA_PRIME"

    try:
        confirmed = client.confirm_eap_authentication(
            challenge["authCtxId"],
            build_eap_aka_prime_reauthentication_payload(),
        )
        print(f"reauth-confirmed: {confirmed}")
        assert confirmed["authResult"] == "ONGOING"
        assert confirmed["message"] == "EAP-AKA' re-authentication challenge generated"
        assert confirmed["eapSession"]["method"] == "EAP-AKA'"
        assert confirmed["eapSession"]["sessionId"] == challenge["authCtxId"]
        assert confirmed["eapSession"]["payload"].startswith("EAP-Request/AKA'-Challenge")

        stored_context = client.get_authentication_context(challenge["authCtxId"])
        assert stored_context["status"] == "CHALLENGE_SENT"
        assert stored_context["eapSession"]["payload"] == confirmed["eapSession"]["payload"]
        print(f"stored-context: {stored_context}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        assert not notifications
        print("amf-notifications: []")

        control_plane_context = load_control_plane_authentication_context(supi)
        eap_payload = build_eap_aka_prime_response_payload(control_plane_context["xresStar"])
        final_confirmed = client.confirm_eap_authentication(challenge["authCtxId"], eap_payload)
        assert final_confirmed["authResult"] == "SUCCESS"
        print(f"final-confirmed: {final_confirmed}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        matching_notification = next(notification for notification in notifications if notification["authCtxId"] == challenge["authCtxId"])
        assert matching_notification["authResult"] == "SUCCESS"
        assert matching_notification["supi"] == supi
        assert matching_notification["authType"] == "EAP_AKA_PRIME"
        assert matching_notification["_requestPath"] == f"/namf-comm/v1/ue-authentications/{challenge['authCtxId']}/status-notify"
        print(f"amf-notification: {matching_notification}")
        return 0
    finally:
        client.delete_authentication_context(challenge["authCtxId"])


if __name__ == "__main__":
    raise SystemExit(main())