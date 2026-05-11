"""smoke_test_sighup_cert_reload.py — SIGHUP TLS certificate hot-reload smoke test.

Scenario:
  1. Bring up compose with docker-compose.tls.yml overlay.
  2. Perform a healthy TLS authentication to confirm AUSF is serving correctly.
  3. Send SIGHUP to the ausf-go container.
  4. Perform a second authentication over TLS to confirm that AUSF continues to
     serve requests normally after the cert-cache flush (the same certificate is
     still on disk, so behaviour must be identical to step 2).
  5. Tear down.

The test verifies that SIGHUP does not crash or stall the process and that TLS
continues to work after the hot-reload.  It does NOT check that a *different*
certificate was loaded — that would require generating a second cert.
"""
from __future__ import annotations

import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = ROOT.parent
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import wait_for_containers
from smoke_client import AUSFClient
from smoke_runtime import (
    AUSF_BASE_URL,
    CONTROL_PLANE_BASE_URL,
    MOCK_AMF_BASE_URL,
    MOCK_NRF_BASE_URL,
    MOCK_UDM_BASE_URL,
    load_amf_notifications,
    load_control_plane_authentication_context,
    wait_for_health,
)

COMPOSE_FILES = ["docker-compose.yml", "docker-compose.tls.yml"]
AUSF_TLS_URL = "https://ausf-go:8080"
CONTROL_PLANE_TLS_URL = "https://ausf-control-plane:8081"
CA_CERT_FILE = "/app/automation/.tls-dev/dev-root-ca.crt"
SUPI = "imsi-250010000000002"
SERVING_NETWORK = "5G:mnc001.mcc001.3gppnetwork.org"
NOTIFY_URI = "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify"


def _compose_run(*args: str, compose_files: list[str] | None = None) -> None:
    command = ["docker", "compose"]
    for cf in (compose_files or COMPOSE_FILES):
        command += ["-f", cf]
    command += list(args)
    print(f"==> {' '.join(command)}", flush=True)
    subprocess.run(command, cwd=REPO_ROOT, check=True)


def _compose_exec_sighup() -> None:
    """Send SIGHUP to PID 1 of the ausf-go container (the AUSF process itself)."""
    print("==> sending SIGHUP to ausf-go PID 1", flush=True)
    subprocess.run(
        ["docker", "compose", "-f", "docker-compose.yml", "-f", "docker-compose.tls.yml",
         "exec", "-T", "ausf-go", "kill", "-HUP", "1"],
        cwd=REPO_ROOT,
        check=True,
    )


def _run_auth_and_confirm(client: AUSFClient, label: str) -> None:
    challenge = client.initiate_authentication(
        SUPI,
        SERVING_NETWORK,
        notification_uri=NOTIFY_URI,
    )
    auth_ctx_id = challenge["authCtxId"]
    print(f"{label}-challenge: {auth_ctx_id}")

    control_plane_context = load_control_plane_authentication_context(SUPI)
    expected_res_star = control_plane_context["xresStar"]

    confirmed = client.confirm_authentication(auth_ctx_id, expected_res_star)
    print(f"{label}-confirmed: {confirmed}")
    assert confirmed.get("authResult") == "AUTHENTICATION_VERIFIED", (
        f"unexpected confirm result: {confirmed}"
    )


def _run_smoke_inside_compose(label: str) -> None:
    """Run a mini smoke via docker compose run (inside the compose network)."""
    env_args: list[str] = []
    for key, value in {
        "AUSF_BASE_URL": AUSF_TLS_URL,
        "CONTROL_PLANE_BASE_URL": CONTROL_PLANE_TLS_URL,
        "MOCK_UDM_BASE_URL": "http://mock-udm:8090",
        "MOCK_NRF_BASE_URL": "http://mock-nrf:8091",
        "MOCK_AMF_BASE_URL": "http://mock-amf:8092",
        "AUSF_CA_CERT_FILE": CA_CERT_FILE,
        "CONTROL_PLANE_CA_CERT_FILE": CA_CERT_FILE,
        "_SIGHUP_LABEL": label,
    }.items():
        env_args += ["-e", f"{key}={value}"]

    _compose_run(
        "run", "--rm", "--no-deps", "-T",
        *env_args,
        "mock-udm",
        "python",
        "automation/scripts/smoke_test_https_tls.py",
    )


def main() -> int:
    # Generate TLS assets if not present (idempotent).
    subprocess.run(
        [sys.executable, "automation/scripts/generate_dev_tls_assets.py"],
        cwd=REPO_ROOT,
        check=True,
    )

    try:
        _compose_run("up", "-d")
        wait_for_containers(["mock-udm", "mock-nrf", "mock-amf"])

        # Phase 1: TLS auth before SIGHUP
        print("==> Phase 1: TLS authentication before SIGHUP")
        _run_smoke_inside_compose("pre-sighup")

        # Send SIGHUP — this should flush the cert cache but not stop AUSF
        _compose_exec_sighup()
        # Give the hot-reload a moment to complete
        time.sleep(1)

        # Phase 2: TLS auth after SIGHUP — must succeed with same cert
        print("==> Phase 2: TLS authentication after SIGHUP")
        _run_smoke_inside_compose("post-sighup")

        print("sighup-cert-reload: SUCCESS")
    finally:
        _compose_run("down", compose_files=COMPOSE_FILES)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
