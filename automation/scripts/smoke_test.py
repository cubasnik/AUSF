from __future__ import annotations

import sys

from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import AUSF_BASE_URL, BASIC_HEALTH_ENDPOINTS, load_control_plane_authentication_context, wait_for_health


def main() -> int:
    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"
    for url in BASIC_HEALTH_ENDPOINTS:
        print(f"health-check: {wait_for_health(url)}")
    client = AUSFClient(AUSF_BASE_URL)
    health = client.health()
    print(f"health: {health}")

    challenge = client.initiate_authentication(supi, serving_network_name)
    control_plane_context = load_control_plane_authentication_context(supi)
    expected_res_star = control_plane_context["xresStar"]
    confirmed = client.confirm_authentication(challenge["authCtxId"], expected_res_star)
    print(f"confirmed: {confirmed}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())