from __future__ import annotations

import sys
from pathlib import Path
import json


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import AUSF_BASE_URL, load_amf_notifications, load_control_plane_authentication_context


def main() -> int:
    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health: {client.health()}")
    print(f"baseline-amf-notification-count: {len(load_amf_notifications())}")

    challenge = client.initiate_authentication(
        supi,
        serving_network_name,
        notification_uri="http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
    )

    control_plane_context = load_control_plane_authentication_context(supi)
    expected_res_star = control_plane_context["xresStar"]
    print(
        "prepared-state: "
        + json.dumps(
            {
                "authCtxId": challenge["authCtxId"],
                "expectedResStar": expected_res_star,
            },
            ensure_ascii=True,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())