from __future__ import annotations

import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PYTHON = sys.executable
GO_IMAGE = "mcr.microsoft.com/devcontainers/go:1-1.22-bookworm"
JAVA_IMAGE = "mcr.microsoft.com/devcontainers/java:25-jdk-bookworm"
GO_TEST_PACKAGES = [
    "./internal/api",
    "./internal/controlplane",
    "./internal/namf",
    "./internal/service",
]
JAVA_TESTS = [
    "AuthenticationManagerTest",
    "AuthenticationControllerTest",
    "NnrfClientTest",
    "HttpUdmClientTest",
]


def run_step(name: str, command: list[str]) -> None:
    print(f"==> {name}", flush=True)
    print(f"    {' '.join(command)}", flush=True)
    subprocess.run(command, cwd=ROOT, check=True)


def main() -> int:
    run_step(
        "Python unit tests",
        [PYTHON, "-m", "unittest", "discover", "-s", "automation/tests"],
    )

    run_step(
        "Go focused tests in container",
        [
            "docker",
            "run",
            "--rm",
            "-v",
            f"{ROOT.resolve()}:/workspace",
            "-w",
            "/workspace/microservices",
            GO_IMAGE,
            "bash",
            "-lc",
            f"go test {' '.join(GO_TEST_PACKAGES)}",
        ],
    )

    run_step(
        "Java focused tests in container",
        [
            "docker",
            "run",
            "--rm",
            "-v",
            f"{(ROOT / 'control-plane').resolve()}:/app",
            "-w",
            "/app",
            JAVA_IMAGE,
            "bash",
            "-lc",
            "rm -f /etc/apt/sources.list.d/yarn.list /etc/apt/sources.list.d/yarn.sources && "
            "apt-get update >/dev/null && "
            "apt-get install -y --no-install-recommends maven >/dev/null && "
            f"mvn -q -Dtest={','.join(JAVA_TESTS)} test",
        ],
    )

    run_step(
        "HTTP UDM happy and negative smoke suite",
        [PYTHON, "automation/scripts/run_http_udm_smoke_suite.py"],
    )

    print("==> full validation completed", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())