from __future__ import annotations

import json
import sys
import time
from http.client import RemoteDisconnected
from pathlib import Path
from urllib import request
from urllib.error import URLError

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError


def wait_for_health(url: str, timeout_seconds: int = 30) -> dict:
    deadline = time.time() + timeout_seconds
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            with request.urlopen(url, timeout=5) as response:
                return json.loads(response.read().decode("utf-8"))
        except (URLError, TimeoutError, json.JSONDecodeError, RemoteDisconnected) as error:
            last_error = error
            time.sleep(1)
    raise RuntimeError(f"timed out waiting for {url}: {last_error}")


def load_amf_notifications() -> list[dict]:
    with request.urlopen("http://127.0.0.1:8092/notifications", timeout=5) as response:
        return json.loads(response.read().decode("utf-8"))["notifications"]


def main() -> int:
    supi = "imsi-250010000000002"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

    print(f"nrf-health: {wait_for_health('http://127.0.0.1:8091/healthz')}")
    print(f"udm-health: {wait_for_health('http://127.0.0.1:8090/healthz')}")
    print(f"amf-health: {wait_for_health('http://127.0.0.1:8092/healthz')}")

    client = AUSFClient("http://127.0.0.1:8080")
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
        client.confirm_eap_authentication(challenge["authCtxId"], "EAP-Response/AKA'-Challenge invalid-token")
    except AUSFError as error:
        assert error.status_code == 401
        assert error.payload["cause"] == "AUTHENTICATION_REJECTED"
        assert error.payload["detail"] == "EAP-AKA' verification failed"
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