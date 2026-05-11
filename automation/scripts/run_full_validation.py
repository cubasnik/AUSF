from __future__ import annotations

import os
import shutil
import subprocess
import sys
import time
from pathlib import Path


AUTOMATION_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AUTOMATION_ROOT / "src"))

from compose_runtime import build_compose_service


ROOT = AUTOMATION_ROOT.parent
PYTHON = sys.executable
GO_IMAGE = "mcr.microsoft.com/devcontainers/go:1-1.22-bookworm"
JAVA_TEST_IMAGE = "ausf-control-plane-test-base:java25"
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
    "SubscriberSeedInitializerTest",
]
FORCE_DOCKER_JAVA_TESTS = os.environ.get("AUSF_FORCE_DOCKER_JAVA_TESTS", "").strip().lower() in {"1", "true", "yes"}


def run_step(name: str, command: list[str], cwd: Path | None = None) -> None:
    print(f"==> {name}", flush=True)
    print(f"    {' '.join(command)}", flush=True)
    started_at = time.monotonic()
    subprocess.run(command, cwd=cwd or ROOT, check=True)
    print(f"    completed in {time.monotonic() - started_at:.1f}s", flush=True)


def run_python_step(name: str, command: list[str]) -> None:
    print(f"==> {name}", flush=True)
    print(f"    {' '.join(command)}", flush=True)
    started_at = time.monotonic()
    subprocess.run(command, cwd=ROOT, check=True)
    print(f"    completed in {time.monotonic() - started_at:.1f}s", flush=True)


def run_action(name: str, action: callable) -> None:
    print(f"==> {name}", flush=True)
    started_at = time.monotonic()
    action()
    print(f"    completed in {time.monotonic() - started_at:.1f}s", flush=True)


def resolve_host_maven() -> str | None:
    for candidate in ("mvn.cmd", "mvn.bat", "mvn"):
        resolved = shutil.which(candidate)
        if resolved is not None:
            return resolved
    return None


def ensure_java_test_image() -> None:
    run_step(
        "Build cached Java test image",
        [
            "docker",
            "build",
            "-t",
            JAVA_TEST_IMAGE,
            "--target",
            "java-test-base",
            "control-plane",
        ],
    )


def run_java_tests() -> None:
    host_maven = resolve_host_maven()
    if not FORCE_DOCKER_JAVA_TESTS and host_maven is not None:
        run_step(
            "Java focused tests on host",
            [
                host_maven,
                "-q",
                f"-Dtest={','.join(JAVA_TESTS)}",
                "test",
            ],
            cwd=ROOT / "control-plane",
        )
        return

    ensure_java_test_image()

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
            JAVA_TEST_IMAGE,
            "mvn",
            "-q",
            f"-Dtest={','.join(JAVA_TESTS)}",
            "test",
        ],
    )


def prepare_runtime_images_for_smoke_suite() -> bool:
    host_maven = resolve_host_maven()
    if host_maven is None or FORCE_DOCKER_JAVA_TESTS:
        return False

    run_step(
        "Package control-plane runtime JAR on host",
        [
            host_maven,
            "-q",
            "-DskipTests",
            "package",
        ],
        cwd=ROOT / "control-plane",
    )

    run_step(
        "Build control-plane runtime image from host JAR",
        [
            "docker",
            "build",
            "-t",
            "ausf-ausf-control-plane:latest",
            "--target",
            "runtime-from-host-jar",
            "control-plane",
        ],
    )

    run_action(
        "Build Go runtime image for smoke suite",
        lambda: build_compose_service("ausf-go"),
    )

    return True


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
            "go",
            "test",
            *GO_TEST_PACKAGES,
        ],
    )

    run_java_tests()

    skip_smoke_build = prepare_runtime_images_for_smoke_suite()
    smoke_command = [PYTHON, "automation/scripts/run_http_udm_smoke_suite.py"]
    if skip_smoke_build:
        smoke_command.append("--skip-build")

    run_python_step(
        "HTTP UDM happy and negative smoke suite",
        smoke_command,
    )

    https_smoke_command = [PYTHON, "automation/scripts/run_https_tls_smoke_suite.py"]
    if skip_smoke_build:
        https_smoke_command.append("--skip-build")

    run_python_step(
        "HTTPS TLS happy and negative smoke suite",
        https_smoke_command,
    )

    run_python_step(
        "SIGHUP TLS certificate hot-reload smoke test",
        [PYTHON, "automation/scripts/smoke_test_sighup_cert_reload.py"],
    )

    run_python_step(
        "Flyway V1 schema migration smoke test",
        [PYTHON, "automation/scripts/smoke_test_flyway_migration.py"],
    )

    # Горизонт 10: Redis Namf queue failover (manages its own compose stack).
    run_python_step(
        "Redis Namf queue failover smoke test",
        [PYTHON, "automation/scripts/smoke_test_redis_failover.py"],
    )

    print("==> full validation completed", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())