from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import AUSF_BASE_URL, MOCK_AMF_BASE_URL, MOCK_NRF_BASE_URL, MOCK_UDM_BASE_URL, load_amf_notifications, load_control_plane_authentication_context, wait_for_health

# ANSI colour helpers
_RESET  = "\033[0m"
_BOLD   = "\033[1m"
_GREEN  = "\033[32m"
_CYAN   = "\033[36m"
_GRAY   = "\033[90m"

def _ok(label: str, value: object) -> None:
    print(f"  {_GREEN}\u2714{_RESET}  {_BOLD}{label:<22}{_RESET} {_GRAY}{value}{_RESET}")

def _section(title: str) -> None:
    print(f"\n{_CYAN}{_BOLD}{chr(0x2500) * 50}{_RESET}")
    print(f"{_CYAN}{_BOLD}  {title}{_RESET}")
    print(f"{_CYAN}{_BOLD}{chr(0x2500) * 50}{_RESET}")


def main() -> int:
    supi = "imsi-250010000000001"
    serving_network_name = "5G:mnc001.mcc001.3gppnetwork.org"

    _section("Health checks")
    _ok("mock-nrf",  wait_for_health(f"{MOCK_NRF_BASE_URL}/healthz"))
    _ok("mock-udm",  wait_for_health(f"{MOCK_UDM_BASE_URL}/healthz"))
    _ok("mock-amf",  wait_for_health(f"{MOCK_AMF_BASE_URL}/healthz"))

    client = AUSFClient(AUSF_BASE_URL)
    _ok("ausf-go",   client.health())

    _section("5G-AKA authentication")
    baseline_notification_count = len(load_amf_notifications())

    challenge = client.initiate_authentication(
        supi,
        serving_network_name,
        notification_uri="http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
    )
    _ok("authCtxId",  challenge["authCtxId"])
    _ok("supi",       supi)

    control_plane_context = load_control_plane_authentication_context(supi)
    expected_res_star = control_plane_context["xresStar"]
    confirmed = client.confirm_authentication(challenge["authCtxId"], expected_res_star)

    auth_result = confirmed.get("authResult", "-")
    result_colour = _GREEN if auth_result == "SUCCESS" else "\033[31m"
    _ok("authResult", f"{result_colour}{_BOLD}{auth_result}{_RESET}")
    _ok("kseaf",      confirmed.get("kseaf", "-"))

    _section("Namf notification")
    notifications = load_amf_notifications()[baseline_notification_count:]
    matching_notification = next(
        n for n in notifications if n["authCtxId"] == challenge["authCtxId"]
    )
    assert matching_notification["authResult"] == "SUCCESS"
    assert matching_notification["supi"] == supi
    assert matching_notification["_requestPath"] == (
        f"/namf-comm/v1/ue-authentications/{challenge['authCtxId']}/status-notify"
    )
    _ok("authResult",  f"{_GREEN}{_BOLD}{matching_notification['authResult']}{_RESET}")
    _ok("supi",        matching_notification["supi"])
    _ok("requestPath", matching_notification["_requestPath"])

    print(f"\n  {_GREEN}{_BOLD}\u2714  All assertions passed{_RESET}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())