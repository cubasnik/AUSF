"""Smoke test: Nausf_UPUProtection (TS 29.509 §6.3).

Flow:
1. Initiate 5G_AKA authentication (supi → challenge).
2. Confirm with xresStar → AUTHENTICATED.
3. PUT /nausf-auth/v1/ue-authentications/{authCtxId}/upu-protection
   with UPU data and ackIndication=false.
4. Verify that the response contains a non-empty upuMacIausf (32 hex chars)
   and a counterUpu (4 hex chars).
"""
from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import (
    AUSF_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    load_control_plane_authentication_context,
    wait_for_health,
)


def main() -> int:
    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"
    # Example UPU data: a routing profile list (TS 33.501 §C.4), 16 bytes as hex
    upu_data = "deadbeefcafe000000000000000000ff"

    print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")

    # Step 1 — initiate
    challenge = client.initiate_authentication(supi, serving_network_name)
    auth_ctx_id = challenge["authCtxId"]
    print(f"challenge: authCtxId={auth_ctx_id}")

    # Step 2 — confirm (5G_AKA)
    cp_ctx = load_control_plane_authentication_context(supi)
    confirmed = client.confirm_authentication(auth_ctx_id, cp_ctx["xresStar"])
    print(f"confirmed: {confirmed}")
    assert confirmed.get("kseaf"), "Expected KSEAF in confirmation response"

    # Step 3 — UPU protection
    upu_result = client.upu_protection(auth_ctx_id, upu_data, ack_indication=False)
    print(f"upu-protection-result: {upu_result}")

    # Step 4 — validate
    mac = upu_result.get("upuMacIausf", "")
    counter = upu_result.get("counterUpu", "")
    assert len(mac) == 32, f"Expected 32-char upuMacIausf but got {len(mac)}: {mac!r}"
    assert len(counter) == 4, f"Expected 4-char counterUpu but got {len(counter)}: {counter!r}"
    assert mac != "0" * 32, "upuMacIausf is all-zeros — possible crypto failure"
    print(f"upu-protection: OK (mac={mac}, counter={counter})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
