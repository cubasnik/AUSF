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
    get_eap_context,
    is_eap_failure,
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
        client.confirm_eap_authentication(
            challenge["authCtxId"],
            build_eap_aka_prime_reauthentication_payload(eap_ctx),
        )
        first_refreshed_control_plane_context = load_control_plane_authentication_context(supi)
        first_refreshed_eap_ctx = get_eap_context(supi, client.get_authentication_context(challenge["authCtxId"])["eapSession"]["payload"])
        stale_payload = build_eap_aka_prime_response_payload(first_refreshed_control_plane_context["xresStar"], first_refreshed_eap_ctx)

        second_refresh = client.confirm_eap_authentication(
            challenge["authCtxId"],
            build_eap_aka_prime_reauthentication_payload(first_refreshed_eap_ctx),
        )
        print(f"second-reauth-confirmed: {second_refresh}")
        assert second_refresh["authResult"] == "ONGOING"

        try:
            client.confirm_eap_authentication(challenge["authCtxId"], stale_payload)
        except AUSFError as error:
            assert error.status_code == 401
            assert error.payload["cause"] == "AUTHENTICATION_REJECTED"
            assert error.payload["detail"] == "EAP-AKA' verification failed"
            assert is_eap_failure(error.payload["eapPayload"])
            print(f"stale-eap-authentication-rejected-error: {error.payload}")
        else:
            raise AssertionError(
                "expected 401 AUTHENTICATION_REJECTED for stale eapPayload after second re-auth refresh"
            )

        stored_context = client.get_authentication_context(challenge["authCtxId"])
        assert stored_context["status"] == "FAILED"
        assert is_eap_failure(stored_context["eapSession"]["payload"])
        print(f"stored-context: {stored_context}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        assert not notifications
        print("amf-notifications: []")
        return 0
    finally:
        client.delete_authentication_context(challenge["authCtxId"])


if __name__ == "__main__":
    raise SystemExit(main())