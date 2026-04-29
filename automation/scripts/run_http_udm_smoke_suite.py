from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


AUTOMATION_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AUTOMATION_ROOT / "src"))

from compose_runtime import DEFAULT_SMOKE_RUNNER_ENV, compose_down, compose_up, run_compose_smoke_script, wait_for_containers


PYTHON = sys.executable
HAPPY_PATH_SCRIPTS = [
    "automation/scripts/smoke_test.py",
    "automation/scripts/smoke_test_http_udm.py",
    "automation/scripts/smoke_test_http_udm_eap.py",
]
NEGATIVE_SCRIPTS = [
    "automation/scripts/smoke_test_http_udm_invalid_notification_uri.py",
    "automation/scripts/smoke_test_http_udm_unsupported_auth_type.py",
    "automation/scripts/smoke_test_http_udm_missing_context.py",
    "automation/scripts/smoke_test_http_udm_missing_subscriber.py",
    "automation/scripts/smoke_test_http_udm_upstream_unavailable.py",
    "automation/scripts/smoke_test_http_udm_authentication_rejected.py",
    "automation/scripts/smoke_test_http_udm_eap_authentication_rejected.py",
]
SMOKE_SCRIPTS = HAPPY_PATH_SCRIPTS + NEGATIVE_SCRIPTS
HOST_LEVEL_SCRIPTS = [
    "automation/scripts/smoke_test_http_udm_context_survives_restart.py",
    "automation/scripts/smoke_test_http_udm_context_ttl_expired.py",
]
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
        for script in HOST_LEVEL_SCRIPTS:
            run_host_smoke_script(script)
    finally:
        compose_down(check=False)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())