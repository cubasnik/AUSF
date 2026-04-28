from __future__ import annotations

import json
import subprocess
import sys
import time
from http.client import RemoteDisconnected
from pathlib import Path
from urllib import request
from urllib.error import URLError


ROOT = Path(__file__).resolve().parents[2]
PYTHON = sys.executable
HAPPY_PATH_SCRIPTS = [
    "automation/scripts/smoke_test.py",
    "automation/scripts/smoke_test_http_udm.py",
    "automation/scripts/smoke_test_http_udm_eap.py",
]
NEGATIVE_SCRIPTS = [
    "automation/scripts/smoke_test_http_udm_invalid_notification_uri.py",
    "automation/scripts/smoke_test_http_udm_missing_context.py",
    "automation/scripts/smoke_test_http_udm_missing_subscriber.py",
    "automation/scripts/smoke_test_http_udm_authentication_rejected.py",
    "automation/scripts/smoke_test_http_udm_eap_authentication_rejected.py",
]
SMOKE_SCRIPTS = HAPPY_PATH_SCRIPTS + NEGATIVE_SCRIPTS
HEALTH_ENDPOINTS = [
    "http://127.0.0.1:8090/healthz",
    "http://127.0.0.1:8091/healthz",
    "http://127.0.0.1:8092/healthz",
    "http://127.0.0.1:8081/healthz",
    "http://127.0.0.1:8080/healthz",
]


def run_step(command: list[str]) -> None:
    print(f"==> {' '.join(command)}", flush=True)
    subprocess.run(command, cwd=ROOT, check=True)


def wait_for_health(url: str, timeout_seconds: int = 60) -> None:
    deadline = time.time() + timeout_seconds
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            with request.urlopen(url, timeout=5) as response:
                payload = json.loads(response.read().decode("utf-8"))
                if payload.get("status") == "ok":
                    print(f"==> healthy {url}", flush=True)
                    return
        except (URLError, TimeoutError, json.JSONDecodeError, RemoteDisconnected, ConnectionResetError, OSError) as error:
            last_error = error
            time.sleep(1)
    raise RuntimeError(f"timed out waiting for {url}: {last_error}")


def main() -> int:
    try:
        run_step(["docker", "compose", "up", "-d"])
        for endpoint in HEALTH_ENDPOINTS:
            wait_for_health(endpoint)
        for script in SMOKE_SCRIPTS:
            run_step([PYTHON, script])
    finally:
        run_step(["docker", "compose", "down"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())