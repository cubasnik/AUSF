from __future__ import annotations

import json
import sys
import time
from http.client import RemoteDisconnected
from pathlib import Path
from urllib import request
from urllib.error import URLError

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError


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


def load_amf_notifications() -> list[dict]:
	with request.urlopen("http://127.0.0.1:8092/notifications", timeout=5) as response:
		return json.loads(response.read().decode("utf-8"))["notifications"]


def main() -> int:
	supi = "imsi-250010000000001"
	serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

	print(f"nrf-health: {wait_for_health('http://127.0.0.1:8091/healthz')}")
	print(f"udm-health: {wait_for_health('http://127.0.0.1:8090/healthz')}")
	print(f"amf-health: {wait_for_health('http://127.0.0.1:8092/healthz')}")

	client = AUSFClient("http://127.0.0.1:8080")
	print(f"ausf-health: {client.health()}")
	baseline_notification_count = len(load_amf_notifications())

	try:
		client.initiate_authentication(
			supi,
			serving_network_name,
			auth_type="AKA_TLS",
			notification_uri="http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
		)
	except AUSFError as error:
		assert error.status_code == 400
		assert error.payload["cause"] == "UNSUPPORTED_AUTH_TYPE"
		assert error.payload["detail"] == "authType must be 5G_AKA or EAP_AKA_PRIME when provided"
		print(f"unsupported-auth-type-error: {error.payload}")
	else:
		raise AssertionError("expected 400 UNSUPPORTED_AUTH_TYPE for unsupported authType")

	notifications = load_amf_notifications()[baseline_notification_count:]
	assert not notifications
	print("amf-notifications: []")
	return 0


if __name__ == "__main__":
	raise SystemExit(main())