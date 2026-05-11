"""Smoke test: Nausf_UPUProtection optional OAS3 fields (TS 29.509 §6.3.6.2.2-1).

Flow:
1. Initiate 5G_AKA authentication (supi → challenge).
2. Confirm with xresStar → AUTHENTICATED.
3a. PUT upu-protection with upuHeader + provisioning3gppInd.
    Verify upuMacIausf differs from the baseline (without upuHeader), confirming
    that upuHeader is fed into the MAC computation.
3b. Repeat for context B (same supi, fresh auth) with no optional fields to get
    a stable baseline for comparison.
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
    upu_data = "deadbeefcafe000000000000000000ff"
    # upuHeader: 16-byte (32 hex) UPU header per TS 33.501 §C.4
    upu_header = "1122334455667788aabbccddeeff0011"

    print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")

    # --- Context A: baseline MAC (no upuHeader) ---
    challenge_a = client.initiate_authentication(supi, serving_network_name)
    auth_ctx_id_a = challenge_a["authCtxId"]
    cp_ctx_a = load_control_plane_authentication_context(supi)
    client.confirm_authentication(auth_ctx_id_a, cp_ctx_a["xresStar"])

    baseline = client.upu_protection(auth_ctx_id_a, upu_data, ack_indication=False)
    mac_baseline = baseline.get("upuMacIausf", "")
    assert len(mac_baseline) == 32, f"Expected 32-char upuMacIausf, got {len(mac_baseline)}: {mac_baseline!r}"
    assert mac_baseline != "0" * 32, "Baseline upuMacIausf is all-zeros — crypto failure"
    print(f"baseline mac (no upuHeader): {mac_baseline}")

    # --- Context B: MAC with upuHeader + provisioning3gppInd ---
    challenge_b = client.initiate_authentication(supi, serving_network_name)
    auth_ctx_id_b = challenge_b["authCtxId"]
    cp_ctx_b = load_control_plane_authentication_context(supi)
    client.confirm_authentication(auth_ctx_id_b, cp_ctx_b["xresStar"])

    result = client.upu_protection(
        auth_ctx_id_b,
        upu_data,
        ack_indication=False,
        upu_header=upu_header,
        provisioning_3gpp_ind=True,
    )
    print(f"upu-protection-optional-fields result: {result}")

    mac_with_header = result.get("upuMacIausf", "")
    assert len(mac_with_header) == 32, f"Expected 32-char upuMacIausf, got {len(mac_with_header)}: {mac_with_header!r}"
    assert mac_with_header != "0" * 32, "upuMacIausf is all-zeros — crypto failure"

    # upuHeader changes the HMAC input → MAC must differ from baseline
    assert mac_with_header != mac_baseline, (
        f"upuMacIausf with upuHeader must differ from baseline (both {mac_with_header!r})"
        " — upuHeader is not being included in the MAC computation"
    )

    print(f"upu-protection optional fields: OK")
    print(f"  mac_baseline={mac_baseline}")
    print(f"  mac_with_header={mac_with_header}  (differs ✓)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
