from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
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
        first_control_plane_context = load_control_plane_authentication_context(supi)
        first_auts = first_control_plane_context["auts"]

        first_resync = client.confirm_authentication_with_auts(challenge["authCtxId"], first_auts)
        print(f"first-sync-failure-confirmed: {first_resync}")
        assert first_resync["authCtxId"] == challenge["authCtxId"]
        assert first_resync["authResult"] == "SYNC_FAILURE"
        assert first_resync["5gAuthData"]["rand"] != challenge["5gAuthData"]["rand"]
        assert first_resync["5gAuthData"]["autn"] != challenge["5gAuthData"]["autn"]

        first_refreshed_context = client.get_authentication_context(challenge["authCtxId"])
        assert first_refreshed_context["status"] == "CHALLENGE_SENT"
        assert first_refreshed_context["5gAuthData"]["rand"] == first_resync["5gAuthData"]["rand"]
        print(f"first-refreshed-context: {first_refreshed_context}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        assert not notifications
        print("amf-notifications-after-first-refresh: []")

        second_control_plane_context = load_control_plane_authentication_context(supi)
        second_auts = second_control_plane_context["auts"]

        second_resync = client.confirm_authentication_with_auts(challenge["authCtxId"], second_auts)
        print(f"second-sync-failure-confirmed: {second_resync}")
        assert second_resync["authCtxId"] == challenge["authCtxId"]
        assert second_resync["authResult"] == "SYNC_FAILURE"
        assert second_resync["5gAuthData"]["rand"] != first_resync["5gAuthData"]["rand"]
        assert second_resync["5gAuthData"]["autn"] != first_resync["5gAuthData"]["autn"]

        second_refreshed_context = client.get_authentication_context(challenge["authCtxId"])
        assert second_refreshed_context["status"] == "CHALLENGE_SENT"
        assert second_refreshed_context["5gAuthData"]["rand"] == second_resync["5gAuthData"]["rand"]
        print(f"second-refreshed-context: {second_refreshed_context}")

        notifications = load_amf_notifications()[baseline_notification_count:]
        assert not notifications
        print("amf-notifications-after-second-refresh: []")

        final_control_plane_context = load_control_plane_authentication_context(supi)
        final_confirmed = client.confirm_authentication(
            challenge["authCtxId"],
            final_control_plane_context["xresStar"],
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
        print(f"amf-notification: {matching_notification}")
        return 0
    finally:
        client.delete_authentication_context(challenge["authCtxId"])


if __name__ == "__main__":
    raise SystemExit(main())