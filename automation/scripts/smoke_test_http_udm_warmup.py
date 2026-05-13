from __future__ import annotations

import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, wait_for_health


def main() -> int:
    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"
    deadline = time.time() + 45
    last_error: Exception | None = None

    print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")
    print(f"amf-health: {wait_for_health(f'{MOCK_AMF_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")

    while time.time() < deadline:
        try:
            challenge = client.initiate_authentication(supi, serving_network_name)
            print(f"warmup-challenge: {challenge}")
            client.delete_authentication_context(challenge["authCtxId"])
            return 0
        except AUSFError as error:
            last_error = error
            if error.status_code in {502, 503}:
                print(f"warmup-retryable-error: {error.payload}")
                time.sleep(1)
                continue
            raise

    raise RuntimeError(f"timed out waiting for authentication readiness: {last_error}")


if __name__ == "__main__":
    raise SystemExit(main())