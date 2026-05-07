from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, EAP_AKA_PRIME_RESPONSE_PREFIX, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, load_amf_notifications, wait_for_health


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
        client.confirm_eap_authentication(challenge["authCtxId"], f"{EAP_AKA_PRIME_RESPONSE_PREFIX}invalid-token")
    except AUSFError as error:
        assert error.status_code == 401
        assert error.payload["cause"] == "AUTHENTICATION_REJECTED"
        assert error.payload["detail"] == "EAP-AKA' verification failed"
        assert error.payload["eapPayload"] == "EAP-Failure"
        print(f"eap-authentication-rejected-error: {error.payload}")
    else:
        raise AssertionError("expected 401 AUTHENTICATION_REJECTED for invalid eapPayload")
    finally:
        client.delete_authentication_context(challenge["authCtxId"])

    notifications = load_amf_notifications()[baseline_notification_count:]
    assert not notifications
    print("amf-notifications: []")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())