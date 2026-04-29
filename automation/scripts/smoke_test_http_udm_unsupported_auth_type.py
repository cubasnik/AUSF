from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, load_amf_notifications, wait_for_health


def main() -> int:
	supi = "imsi-250010000000001"
	serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

	print(f"nrf-health: {wait_for_health(f'{MOCK_NRF_BASE_URL}/healthz')}")
	print(f"udm-health: {wait_for_health(f'{MOCK_UDM_BASE_URL}/healthz')}")
	print(f"amf-health: {wait_for_health(f'{MOCK_AMF_BASE_URL}/healthz')}")

	client = AUSFClient(AUSF_BASE_URL)
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