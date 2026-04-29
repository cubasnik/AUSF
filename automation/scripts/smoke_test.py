from __future__ import annotations

import hashlib
import json
import sys

from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import AUSF_BASE_URL, BASIC_HEALTH_ENDPOINTS, wait_for_health


def load_permanent_key(supi: str) -> str:
    subscribers_path = ROOT.parent / "control-plane" / "data" / "subscribers.json"
    subscribers = json.loads(subscribers_path.read_text(encoding="utf-8"))
    for subscriber in subscribers:
        if subscriber["supi"] == supi:
            return subscriber["permanentKey"]
    raise ValueError(f"seed subscriber not found for {supi}")


def main() -> int:
    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"
    for url in BASIC_HEALTH_ENDPOINTS:
        print(f"health-check: {wait_for_health(url)}")
    client = AUSFClient(AUSF_BASE_URL)
    health = client.health()
    print(f"health: {health}")

    challenge = client.initiate_authentication(supi, serving_network_name)
    auth_data = challenge["5gAuthData"]
    permanent_key = load_permanent_key(supi)
    expected_res_star = hashlib.sha256(f"{auth_data['rand']}{auth_data['autn']}{permanent_key}".encode("utf-8")).hexdigest()[:32]
    confirmed = client.confirm_authentication(challenge["authCtxId"], expected_res_star)
    print(f"confirmed: {confirmed}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())