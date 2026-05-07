from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, AUSF_CA_CERT_FILE


def main() -> int:
    if not AUSF_CA_CERT_FILE:
        raise RuntimeError("AUSF_CA_CERT_FILE must be set for the HTTPS TLS invalid-CA smoke test")

    client = AUSFClient(AUSF_BASE_URL, ca_cert_file=AUSF_CA_CERT_FILE)
    health = client.health()
    print(f"ausf-health: {health}")

    try:
        client.initiate_authentication(
            "imsi-250010000000001",
            "5G:mnc001.mcc001.3gppnetwork.org",
        )
    except AUSFError as error:
        assert error.status_code in {502, 503}
        assert error.payload["cause"] == "CONTROL_PLANE_UNAVAILABLE"
        print(f"expected-error: {error.status_code} {error.payload}")
        return 0

    raise AssertionError("expected initiate_authentication() to fail when ausf-go trusts the wrong control-plane CA")


if __name__ == "__main__":
    raise SystemExit(main())