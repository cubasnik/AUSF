"""Smoke test: SUCI null-scheme de-concealment (TS 33.501 §C.3.4.1).

When the UE sends a SUCI with protection scheme 0 (null-scheme), the AUSF must
treat the scheme output as the plain MSIN and reconstruct a SUPI of the form
imsi-<mcc><mnc><msin> without any crypto operation.

The mock UDM subscriber database contains imsi-250010000000001 with mcc=250,
mnc=01.  A null-scheme SUCI for this subscriber is:

    suci-0-250-01-0000-0-0-0000000001

where:
  suci    = literal "suci"
  supiType = 0 (IMSI)
  mcc     = 250
  mnc     = 01
  ri      = 0000 (routing indicator)
  schemeId = 0 (null-scheme)
  keyId   = 0
  schemeOutput = 0000000001  (the plain MSIN)
"""
from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import AUSF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, wait_for_health


def main() -> int:
    suci = "suci-0-250-01-0000-0-0-0000000001"
    expected_supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

    print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
    print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")

    # Initiate authentication with a SUCI instead of a plain SUPI.
    # The AUSF must de-conceal it and forward the resolved SUPI to UDM.
    challenge = client.initiate_authentication(suci, serving_network_name)
    print(f"challenge: {challenge}")

    assert challenge.get("supi") == expected_supi, (
        f"Expected SUPI {expected_supi!r} after SUCI de-concealment but got {challenge.get('supi')!r}"
    )
    assert challenge.get("authType") == "5G_AKA", (
        f"Expected 5G_AKA auth type but got {challenge.get('authType')!r}"
    )
    assert "authCtxId" in challenge, "Expected authCtxId in challenge response"
    print(f"suci-null-scheme-deconcealment: OK (supi={expected_supi})")

    client.delete_authentication_context(challenge["authCtxId"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
