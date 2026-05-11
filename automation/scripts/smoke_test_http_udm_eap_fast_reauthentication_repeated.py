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
    build_eap_aka_prime_fast_reauthentication_payload,
    build_eap_aka_prime_response_payload,
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

    try:
        eap_ctx = get_eap_context(supi, challenge["eapSession"]["payload"])
        first_reauth = client.confirm_eap_authentication(
            challenge["authCtxId"],
            build_eap_aka_prime_fast_reauthentication_payload(eap_ctx),
        )
        print(f"first-fast-reauth-confirmed: {first_reauth}")
        assert first_reauth["authCtxId"] == challenge["authCtxId"]
        assert first_reauth["authResult"] == "ONGOING"
        assert first_reauth["eapSession"]["payload"] != challenge["eapSession"]["payload"]

        first_refreshed_context = client.get_authentication_context(challenge["authCtxId"])
        assert first_refreshed_context["status"] == "CHALLENGE_SENT"
        assert first_refreshed_context["eapSession"]["payload"] == first_reauth["eapSession"]["payload"]
        print(f"first-refreshed-context: {first_refreshed_context}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        assert not notifications
        print("amf-notifications-after-first-refresh: []")

        eap_ctx2 = get_eap_context(supi, first_reauth["eapSession"]["payload"])
        second_reauth = client.confirm_eap_authentication(
            challenge["authCtxId"],
            build_eap_aka_prime_fast_reauthentication_payload(eap_ctx2),
        )
        print(f"second-fast-reauth-confirmed: {second_reauth}")
        assert second_reauth["authCtxId"] == challenge["authCtxId"]
        assert second_reauth["authResult"] == "ONGOING"
        assert second_reauth["eapSession"]["payload"] != first_reauth["eapSession"]["payload"]

        second_refreshed_context = client.get_authentication_context(challenge["authCtxId"])
        assert second_refreshed_context["status"] == "CHALLENGE_SENT"
        assert second_refreshed_context["eapSession"]["payload"] == second_reauth["eapSession"]["payload"]
        print(f"second-refreshed-context: {second_refreshed_context}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        assert not notifications
        print("amf-notifications-after-second-refresh: []")

        final_control_plane_context = load_control_plane_authentication_context(supi)
        final_eap_ctx = get_eap_context(supi, second_reauth["eapSession"]["payload"])
        final_confirmed = client.confirm_eap_authentication(
            challenge["authCtxId"],
            build_eap_aka_prime_response_payload(final_control_plane_context["xresStar"], final_eap_ctx),
        )
        print(f"final-confirmed: {final_confirmed}")
        assert final_confirmed["authCtxId"] == challenge["authCtxId"]
        assert final_confirmed["authResult"] == "SUCCESS"
        assert final_confirmed["kseaf"]

        final_context = client.get_authentication_context(challenge["authCtxId"])
        assert final_context["status"] == "AUTHENTICATED"
        print(f"final-context: {final_context}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        matching_notification = next(notification for notification in notifications if notification["authCtxId"] == challenge["authCtxId"])
        assert matching_notification["authResult"] == "SUCCESS"
        assert matching_notification["authType"] == "EAP_AKA_PRIME"
        print(f"amf-notification: {matching_notification}")
        return 0
    finally:
        client.delete_authentication_context(challenge["authCtxId"])


if __name__ == "__main__":
    raise SystemExit(main())