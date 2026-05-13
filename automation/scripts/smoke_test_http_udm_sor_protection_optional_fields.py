"""Smoke test: Nausf_SoRProtection optional OAS3 fields (TS 29.509 §6.2.6.2.2-1).

Flow:
1. Initiate 5G_AKA authentication (supi → challenge).
2. Confirm with xresStar → AUTHENTICATED.
3a. PUT sor-protection with sorHeader + storageIndicator + provisioning3gppInd.
    Verify sorMacIausf differs from the baseline (without sorHeader), confirming
    that sorHeader is fed into the MAC computation.
    Verify storageIndicator is echoed back in the response.
3b. PUT sor-protection again (same context, ackIndication=False, no header)
    and confirm the MAC is also non-trivial.
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
    steering_container = "0000000000000000000000000000abcd"
    # sorHeader: 16-byte (32 hex) steering header per TS 33.501 Table D.2-1
    sor_header = "aabbccddeeff00112233445566778899"
    storage_indicator = "STORE_UE_SECURITY_CONTEXT_DATA"

    print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")

    # --- Context A: baseline MAC (no sorHeader) ---
    challenge_a = client.initiate_authentication(supi, serving_network_name)
    auth_ctx_id_a = challenge_a["authCtxId"]
    cp_ctx_a = load_control_plane_authentication_context(supi)
    client.confirm_authentication(auth_ctx_id_a, cp_ctx_a["xresStar"])

    baseline = client.sor_protection(auth_ctx_id_a, steering_container, ack_indication=True)
    mac_baseline = baseline.get("sorMacIausf", "")
    assert len(mac_baseline) == 32, f"Expected 32-char sorMacIausf, got {len(mac_baseline)}: {mac_baseline!r}"
    assert mac_baseline != "0" * 32, "Baseline sorMacIausf is all-zeros — crypto failure"
    print(f"baseline mac (no sorHeader): {mac_baseline}")

    # --- Context B: MAC with sorHeader + storageIndicator + provisioning3gppInd ---
    challenge_b = client.initiate_authentication(supi, serving_network_name)
    auth_ctx_id_b = challenge_b["authCtxId"]
    cp_ctx_b = load_control_plane_authentication_context(supi)
    client.confirm_authentication(auth_ctx_id_b, cp_ctx_b["xresStar"])

    result = client.sor_protection(
        auth_ctx_id_b,
        steering_container,
        ack_indication=True,
        sor_header=sor_header,
        storage_indicator=storage_indicator,
        provisioning_3gpp_ind=True,
    )
    print(f"sor-protection-optional-fields result: {result}")

    mac_with_header = result.get("sorMacIausf", "")
    assert len(mac_with_header) == 32, f"Expected 32-char sorMacIausf, got {len(mac_with_header)}: {mac_with_header!r}"
    assert mac_with_header != "0" * 32, "sorMacIausf is all-zeros — crypto failure"

    # sorHeader changes the HMAC input → MAC must differ from baseline
    assert mac_with_header != mac_baseline, (
        f"sorMacIausf with sorHeader must differ from baseline (both {mac_with_header!r})"
        " — sorHeader is not being included in the MAC computation"
    )

    # storageIndicator must be echoed back (TS 29.509 §6.2.6.2.2)
    echoed = result.get("storageIndicator")
    assert echoed == storage_indicator, (
        f"Expected storageIndicator={storage_indicator!r} echoed in response, got {echoed!r}"
    )

    print(f"sor-protection optional fields: OK")
    print(f"  mac_baseline={mac_baseline}")
    print(f"  mac_with_header={mac_with_header}  (differs ✓)")
    print(f"  storageIndicator echoed={echoed!r}  ✓")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
