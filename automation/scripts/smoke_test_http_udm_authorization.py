from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, wait_for_health


def main() -> int:
    bearer_token = os.environ.get("AUSF_BEARER_TOKEN", "")
    if not bearer_token:
        print("SKIP: AUSF_BEARER_TOKEN not set — skipping authorization smoke test", flush=True)
        return 0

    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

    print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")

    unauthorized_client = AUSFClient(AUSF_BASE_URL, bearer_token="")
    try:
        unauthorized_client.initiate_authentication(supi, serving_network_name)
    except AUSFError as error:
        assert error.status_code == 401
        assert error.payload["cause"] == "UNAUTHORIZED"
        print(f"unauthorized-error: {error.payload}")
    else:
        raise AssertionError("expected 401 UNAUTHORIZED when bearer token is missing")

    authorized_client = AUSFClient(AUSF_BASE_URL, bearer_token=bearer_token)
    print(f"ausf-health: {authorized_client.health()}")
    challenge = authorized_client.initiate_authentication(supi, serving_network_name)
    assert challenge["authType"] == "5G_AKA"
    print(f"authorized-challenge: {challenge}")
    authorized_client.delete_authentication_context(challenge["authCtxId"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())