from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import restart_compose_service, wait_for_container_health
from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, load_amf_notifications, wait_for_health


STORE_PATH = "/tmp/ausf-auth-contexts.json"


def read_auth_context_store() -> dict:
    result = subprocess.run(
        ["docker", "compose", "exec", "-T", "ausf-go", "sh", "-lc", f"cat {STORE_PATH}"],
        cwd=ROOT.parent,
        check=True,
        capture_output=True,
        text=True,
    )
    return json.loads(result.stdout)


def write_auth_context_store(payload: dict) -> None:
    raw = json.dumps(payload, ensure_ascii=True)
    subprocess.run(
        ["docker", "compose", "exec", "-T", "ausf-go", "sh", "-lc", f"cat > {STORE_PATH}"],
        cwd=ROOT.parent,
        input=raw,
        text=True,
        check=True,
    )


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

    store = read_auth_context_store()
    contexts = store.get("contexts", {})
    context = contexts.get(challenge["authCtxId"])
    if context is None:
        raise AssertionError(f"auth context {challenge['authCtxId']} not found in persisted store")

    # Force expiration without waiting for TTL by backdating the persisted timestamp.
    context["createdAt"] = "2000-01-01T00:00:00Z"
    write_auth_context_store(store)

    restart_compose_service("ausf-go")
    wait_for_container_health("ausf-go")

    try:
        client.get_authentication_context(challenge["authCtxId"])
    except AUSFError as error:
        assert error.status_code == 404
        assert error.payload["cause"] == "CONTEXT_NOT_FOUND"
        assert error.payload["detail"] == "authentication context not found"
        print(f"ttl-expired-context-error: {error.payload}")
    else:
        raise AssertionError("expected 404 CONTEXT_NOT_FOUND for expired authentication context")

    store_after = read_auth_context_store()
    assert challenge["authCtxId"] not in store_after.get("contexts", {})
    print("expired-context-cleanup: removed from persisted auth context store")

    notifications = load_amf_notifications()[baseline_notification_count:]
    assert not notifications
    print("amf-notifications: []")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())