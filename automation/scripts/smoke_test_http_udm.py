from __future__ import annotations

import hashlib
import json
import sys
import time
from http.client import RemoteDisconnected

from pathlib import Path
from urllib import request
from urllib.error import URLError

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient


def load_permanent_key(supi: str) -> str:
    subscribers_path = ROOT.parent / "control-plane" / "data" / "subscribers.json"
    subscribers = json.loads(subscribers_path.read_text(encoding="utf-8"))
    for subscriber in subscribers:
        if subscriber["supi"] == supi:
            return subscriber["permanentKey"]
    raise ValueError(f"seed subscriber not found for {supi}")


def wait_for_health(url: str, timeout_seconds: int = 30) -> dict:
    deadline = time.time() + timeout_seconds
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            with request.urlopen(url, timeout=5) as response:
                return json.loads(response.read().decode("utf-8"))
        except (URLError, TimeoutError, json.JSONDecodeError, RemoteDisconnected) as error:
            last_error = error
            time.sleep(1)
    raise RuntimeError(f"timed out waiting for {url}: {last_error}")


def main() -> int:
    supi = "imsi-001010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

    print(f"nrf-health: {wait_for_health('http://127.0.0.1:8091/healthz')}")
    print(f"udm-health: {wait_for_health('http://127.0.0.1:8090/healthz')}")

    client = AUSFClient("http://127.0.0.1:8080")
    print(f"ausf-health: {client.health()}")

    challenge = client.initiate_authentication(supi, serving_network_name)
    auth_data = challenge["5gAuthData"]
    permanent_key = load_permanent_key(supi)
    expected_res_star = hashlib.sha256(f"{auth_data['rand']}{auth_data['autn']}{permanent_key}".encode("utf-8")).hexdigest()[:32]
    confirmed = client.confirm_authentication(challenge["authCtxId"], expected_res_star)
    print(f"confirmed: {confirmed}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())