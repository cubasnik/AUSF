from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


AUTOMATION_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AUTOMATION_ROOT / "src"))

from compose_runtime import DEFAULT_SMOKE_RUNNER_ENV, compose_down, compose_up, restart_compose_service, run_compose_smoke_script, wait_for_containers


PYTHON = sys.executable
HAPPY_PATH_SCRIPTS = [
    "automation/scripts/smoke_test_http_udm_warmup.py",
    "automation/scripts/smoke_test_http_udm_sync_failure.py",
    "automation/scripts/smoke_test.py",
    "automation/scripts/smoke_test_http_udm.py",
    "automation/scripts/smoke_test_http_udm_eap.py",
    "automation/scripts/smoke_test_http_udm_eap_sync_failure.py",
    "automation/scripts/smoke_test_http_udm_eap_reauthentication_ongoing.py",
    "automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_ongoing.py",
]
NEGATIVE_SCRIPTS = [
    "automation/scripts/smoke_test_http_udm_invalid_notification_uri.py",
    "automation/scripts/smoke_test_http_udm_unsupported_auth_type.py",
    "automation/scripts/smoke_test_http_udm_missing_context.py",
    "automation/scripts/smoke_test_http_udm_missing_subscriber.py",
    "automation/scripts/smoke_test_http_udm_upstream_unavailable.py",
    "automation/scripts/smoke_test_http_udm_authentication_rejected.py",
    "automation/scripts/smoke_test_http_udm_invalid_auts.py",
    "automation/scripts/smoke_test_http_udm_5g_aka_missing_confirmation_payload.py",
    "automation/scripts/smoke_test_http_udm_5g_aka_eap_payload_rejected.py",
    "automation/scripts/smoke_test_http_udm_ambiguous_confirmation_payload.py",
    "automation/scripts/smoke_test_http_udm_repeat_confirm_after_failure.py",
    "automation/scripts/smoke_test_http_udm_repeat_auts_after_failure.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_after_success.py",
    "automation/scripts/smoke_test_http_udm_repeat_confirm_after_sync_failure_success.py",
    "automation/scripts/smoke_test_http_udm_repeat_confirm_after_sync_failure_failure.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_after_failure.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_repeated_valid_auts.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_third_valid_auts.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_fourth_valid_auts.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_fourth_stale_auts.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_third_stale_auts.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_third_stale_response.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_stale_response.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_stale_auts.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_repeated_stale_auts.py",
    "automation/scripts/smoke_test_http_udm_sync_failure_repeated_stale_response.py",
    "automation/scripts/smoke_test_http_udm_eap_authentication_rejected.py",
    "automation/scripts/smoke_test_http_udm_repeat_confirm_after_success.py",
    "automation/scripts/smoke_test_http_udm_repeat_auts_after_success.py",
    "automation/scripts/smoke_test_http_udm_eap_repeat_confirm_after_failure.py",
    "automation/scripts/smoke_test_http_udm_eap_repeat_confirm_after_success.py",
    "automation/scripts/smoke_test_http_udm_eap_sync_failure_after_success.py",
    "automation/scripts/smoke_test_http_udm_eap_sync_failure_after_failure.py",
    "automation/scripts/smoke_test_http_udm_eap_reauthentication_after_success.py",
    "automation/scripts/smoke_test_http_udm_eap_reauthentication_after_failure.py",
    "automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_after_success.py",
    "automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_after_failure.py",
    "automation/scripts/smoke_test_http_udm_eap_sync_failure_repeated.py",
    "automation/scripts/smoke_test_http_udm_eap_reauthentication_repeated.py",
    "automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_repeated.py",
    "automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_repeated_stale_response.py",
    "automation/scripts/smoke_test_http_udm_eap_reauthentication_repeated_stale_response.py",
    "automation/scripts/smoke_test_http_udm_eap_sync_failure_repeated_stale_response.py",
    "automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_stale_response.py",
    "automation/scripts/smoke_test_http_udm_eap_reauthentication_stale_response.py",
    "automation/scripts/smoke_test_http_udm_eap_sync_failure_stale_response.py",
]
SMOKE_SCRIPTS = HAPPY_PATH_SCRIPTS + NEGATIVE_SCRIPTS
HOST_LEVEL_SCRIPTS = [
    "automation/scripts/smoke_test_http_udm_context_survives_restart.py",
    "automation/scripts/smoke_test_http_udm_context_ttl_expired.py",
]
RESET_SERVICES_AFTER_SCRIPT = {
    "automation/scripts/smoke_test_http_udm_upstream_unavailable.py": ["ausf-control-plane"],
}
HEALTH_CONTAINERS = [
    "mock-udm",
    "mock-nrf",
    "mock-amf",
    "ausf-control-plane",
    "ausf-go",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run the HTTP UDM smoke validation suite.")
    parser.add_argument(
        "--skip-build",
        action="store_true",
        help="Reuse already-built images and start compose without --build.",
    )
    return parser.parse_args()


def run_host_smoke_script(script: str) -> None:
    command = [PYTHON, script]
    print(f"==> {' '.join(command)}", flush=True)
    subprocess.run(command, cwd=AUTOMATION_ROOT.parent, check=True)


def main() -> int:
    args = parse_args()
    try:
        compose_up(skip_build=args.skip_build)
        wait_for_containers(HEALTH_CONTAINERS)
        for script in SMOKE_SCRIPTS:
            run_compose_smoke_script(script, env=DEFAULT_SMOKE_RUNNER_ENV)
            for service_name in RESET_SERVICES_AFTER_SCRIPT.get(script, []):
                restart_compose_service(service_name)
                wait_for_containers([service_name])
        for script in HOST_LEVEL_SCRIPTS:
            run_host_smoke_script(script)
    finally:
        compose_down(check=False)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())