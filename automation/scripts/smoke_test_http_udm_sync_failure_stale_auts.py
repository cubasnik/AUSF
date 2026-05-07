from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, load_amf_notifications, load_control_plane_authentication_context, wait_for_health


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

    try:
        initial_control_plane_context = load_control_plane_authentication_context(supi)
        auts = initial_control_plane_context["auts"]
        refreshed = client.confirm_authentication_with_auts(challenge["authCtxId"], auts)
        print(f"sync-failure-confirmed: {refreshed}")

        refreshed_control_plane_context = load_control_plane_authentication_context(supi)
        print(f"refreshed-control-plane-context: {refreshed_control_plane_context}")

        try:
            unexpected = client.confirm_authentication_with_auts(challenge["authCtxId"], auts)
            print(f"unexpected-second-confirmation: {unexpected}")
        except AUSFError as error:
            assert error.status_code == 401
            assert error.payload["cause"] == "AUTHENTICATION_REJECTED"
            assert error.payload["detail"] == "AUTS verification failed"
            print(f"stale-auts-authentication-rejected-error: {error.payload}")
        else:
            raise AssertionError("expected 401 AUTHENTICATION_REJECTED for stale AUTS after 5G_AKA re-sync refresh")

        stored_context = client.get_authentication_context(challenge["authCtxId"])
        assert stored_context["status"] == "FAILED"
        assert stored_context["5gAuthData"]["rand"] == refreshed["5gAuthData"]["rand"]
        print(f"stored-context: {stored_context}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        assert not notifications
        print("amf-notifications: []")
        return 0
    finally:
        client.delete_authentication_context(challenge["authCtxId"])


if __name__ == "__main__":
    raise SystemExit(main())