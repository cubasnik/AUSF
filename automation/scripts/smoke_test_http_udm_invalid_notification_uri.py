from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, load_amf_notifications, wait_for_health


def main() -> int:
    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

    print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")
    print(f"amf-health: {wait_for_health(f'{MOCK_AMF_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")
    baseline_notification_count = len(load_amf_notifications())

    try:
        client.initiate_authentication(
            supi,
            serving_network_name,
            notification_uri="/relative/callback",
        )
    except AUSFError as error:
        assert error.status_code == 400
        assert error.payload["cause"] == "INVALID_NOTIFICATION_URI"
        assert error.payload["detail"] == "notificationUri must be an absolute http or https URL"
        print(f"invalid-notification-uri-error: {error.payload}")
    else:
        raise AssertionError("expected 400 INVALID_NOTIFICATION_URI for relative notificationUri")

    notifications = load_amf_notifications()[baseline_notification_count:]
    assert not notifications
    print("amf-notifications: []")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())