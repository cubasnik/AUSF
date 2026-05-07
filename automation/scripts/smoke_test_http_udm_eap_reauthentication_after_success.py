from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import (
    AUSF_BASE_URL,
    MOCK_AMF_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    build_eap_aka_prime_reauthentication_payload,
    build_eap_aka_prime_response_payload,
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
        refreshed = client.confirm_eap_authentication(
            challenge["authCtxId"],
            build_eap_aka_prime_reauthentication_payload(),
        )
        print(f"reauth-confirmed: {refreshed}")
        assert refreshed["authResult"] == "ONGOING"

        control_plane_context = load_control_plane_authentication_context(supi)
        confirmed = client.confirm_eap_authentication(
            challenge["authCtxId"],
            build_eap_aka_prime_response_payload(control_plane_context["xresStar"]),
        )
        print(f"confirmed: {confirmed}")
        assert confirmed["authResult"] == "SUCCESS"

        notifications = load_amf_notifications()[baseline_notification_count:]
        matching_notification = next(
            notification
            for notification in notifications
            if notification["authCtxId"] == challenge["authCtxId"]
        )
        assert matching_notification["authResult"] == "SUCCESS"
        assert matching_notification["authType"] == "EAP_AKA_PRIME"
        print(f"amf-notification: {matching_notification}")

        try:
            client.confirm_eap_authentication(
                challenge["authCtxId"],
                build_eap_aka_prime_reauthentication_payload(),
            )
        except AUSFError as error:
            assert error.status_code == 401
            assert error.payload["cause"] == "AUTHENTICATION_REJECTED"
            assert error.payload["detail"] == "authentication context is no longer pending"
            assert "eapPayload" not in error.payload
            print(f"repeat-reauth-rejected-error: {error.payload}")
        else:
            raise AssertionError(
                "expected 401 AUTHENTICATION_REJECTED for re-authentication after success"
            )

        stored_context = client.get_authentication_context(challenge["authCtxId"])
        assert stored_context["status"] == "AUTHENTICATED"
        notifications = load_amf_notifications()[baseline_notification_count:]
        assert len(
            [notification for notification in notifications if notification["authCtxId"] == challenge["authCtxId"]]
        ) == 1
        print(f"stored-context: {stored_context}")
        return 0
    finally:
        client.delete_authentication_context(challenge["authCtxId"])


if __name__ == "__main__":
    raise SystemExit(main())