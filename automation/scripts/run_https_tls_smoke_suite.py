from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path

AUTOMATION_ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = AUTOMATION_ROOT.parent
PYTHON = sys.executable
COMPOSE_FILES = ["docker-compose.yml", "docker-compose.tls.yml"]
NEGATIVE_COMPOSE_FILES = [*COMPOSE_FILES, "docker-compose.tls.invalid-control-plane-ca.yml"]

sys.path.insert(0, str(AUTOMATION_ROOT / "src"))
from compose_runtime import wait_for_containers  # noqa: E402


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run the HTTPS/TLS compose smoke suite.")
    parser.add_argument("--skip-build", action="store_true", help="Reuse already-built images and start compose without --build.")
    parser.add_argument("--force-certs", action="store_true", help="Regenerate TLS assets before running the suite.")
    return parser.parse_args()


def run_compose(arguments: list[str], compose_files: list[str] | None = None, check: bool = True) -> None:
    command = ["docker", "compose"]
    for compose_file in compose_files or COMPOSE_FILES:
        command.extend(["-f", compose_file])
    command.extend(arguments)
    print(f"==> {' '.join(command)}", flush=True)
    subprocess.run(command, cwd=REPO_ROOT, check=check)


def compose_up_tls(skip_build: bool, compose_files: list[str] | None = None) -> None:
    arguments = ["up", "-d"]
    if not skip_build:
        arguments.insert(1, "--build")
    run_compose(arguments, compose_files=compose_files)


def compose_down_tls(compose_files: list[str] | None = None, check: bool = False) -> None:
    run_compose(["down"], compose_files=compose_files, check=check)


def generate_certs(force: bool) -> None:
    command = [PYTHON, "automation/scripts/generate_dev_tls_assets.py"]
    if force:
        command.append("--force")
    print(f"==> {' '.join(command)}", flush=True)
    subprocess.run(command, cwd=REPO_ROOT, check=True)


def common_smoke_run_prefix() -> list[str]:
    ca_file = "/app/automation/.tls-dev/dev-root-ca.crt"
    return [
        "run", "--rm", "--no-deps", "-T",
        "-e", "AUSF_BASE_URL=https://ausf-go:8080",
        "-e", "CONTROL_PLANE_BASE_URL=https://ausf-control-plane:8081",
        "-e", "MOCK_UDM_BASE_URL=http://mock-udm:8090",
        "-e", "MOCK_NRF_BASE_URL=http://mock-nrf:8091",
        "-e", "MOCK_AMF_BASE_URL=http://mock-amf:8092",
        "-e", f"AUSF_CA_CERT_FILE={ca_file}",
        "-e", f"CONTROL_PLANE_CA_CERT_FILE={ca_file}",
        "mock-udm",
        "python",
    ]


def run_positive_compose_smoke_scripts() -> None:
    common_arguments = common_smoke_run_prefix()
    run_compose([*common_arguments, "automation/scripts/smoke_test_https_tls.py"])
    run_compose([*common_arguments, "automation/scripts/smoke_test_https_tls_eap.py"])


def run_negative_compose_smoke_script() -> None:
    common_arguments = common_smoke_run_prefix()
    run_compose(
        [*common_arguments, "automation/scripts/smoke_test_https_tls_invalid_control_plane_ca.py"],
        compose_files=NEGATIVE_COMPOSE_FILES,
    )


def run_phase(skip_build: bool, compose_files: list[str], smoke_action: callable) -> None:
    try:
        compose_up_tls(skip_build=skip_build, compose_files=compose_files)
        wait_for_containers(["mock-udm", "mock-nrf", "mock-amf"])
        smoke_action()
    finally:
        compose_down_tls(compose_files=compose_files, check=False)


def main() -> int:
    args = parse_args()
    generate_certs(force=args.force_certs)
    run_phase(args.skip_build, COMPOSE_FILES, run_positive_compose_smoke_scripts)
    run_phase(args.skip_build, NEGATIVE_COMPOSE_FILES, run_negative_compose_smoke_script)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
