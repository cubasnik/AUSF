from __future__ import annotations

import subprocess
import time
from collections.abc import Iterable, Mapping, Sequence
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_SMOKE_RUNNER_ENV = {
    "AUSF_BASE_URL": "http://ausf-go:8080",
    "CONTROL_PLANE_BASE_URL": "http://ausf-control-plane:8081",
    "MOCK_UDM_BASE_URL": "http://mock-udm:8090",
    "MOCK_NRF_BASE_URL": "http://mock-nrf:8091",
    "MOCK_AMF_BASE_URL": "http://mock-amf:8092",
}


def run_compose_command(arguments: Sequence[str], check: bool = True) -> None:
    run_compose_subprocess(arguments, check=check)


def run_compose_subprocess(
    arguments: Sequence[str],
    check: bool = True,
    capture_output: bool = False,
    text: bool = False,
    input: str | None = None,
) -> subprocess.CompletedProcess:
    command = ["docker", "compose", *arguments]
    print(f"==> {' '.join(command)}", flush=True)
    return subprocess.run(
        command,
        cwd=ROOT,
        check=check,
        capture_output=capture_output,
        text=text,
        input=input,
    )


def compose_up(skip_build: bool = False) -> None:
    arguments = ["up", "-d"]
    if not skip_build:
        arguments.insert(1, "--build")
    run_compose_command(arguments)


def compose_down(check: bool = False) -> None:
    run_compose_command(["down"], check=check)


def build_compose_service(service_name: str) -> None:
    run_compose_command(["build", service_name])


def restart_compose_service(service_name: str) -> None:
    run_compose_command(["restart", service_name])


def read_container_health(container_name: str) -> str:
    result = subprocess.run(
        [
            "docker",
            "inspect",
            "--format",
            "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}",
            container_name,
        ],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout.strip()


LOGS_DIR = ROOT / "ci-container-logs"


def dump_container_logs(container_name: str) -> None:
    print(f"==> logs for {container_name}:", flush=True)
    result = subprocess.run(
        ["docker", "logs", "--tail", "300", container_name],
        cwd=ROOT,
        check=False,
        capture_output=True,
        text=True,
    )
    output = result.stdout + result.stderr
    print(output, flush=True)
    # Also save to file so CI can upload as artifact
    LOGS_DIR.mkdir(parents=True, exist_ok=True)
    (LOGS_DIR / f"{container_name}.log").write_text(output, encoding="utf-8")


def wait_for_container_health(container_name: str, timeout_seconds: int = 120) -> None:
    deadline = time.time() + timeout_seconds
    unhealthy_since: float | None = None
    last_log_time: float = 0.0
    last_error: Exception | None = None
    while time.time() < deadline:
        now = time.time()
        try:
            status = read_container_health(container_name)
            if status == "healthy":
                print(f"==> healthy {container_name}", flush=True)
                return
            if status in {"exited", "dead"}:
                dump_container_logs(container_name)
                _dump_docker_ps()
                raise RuntimeError(f"container {container_name} entered status {status}")
            if status == "unhealthy":
                if unhealthy_since is None:
                    unhealthy_since = now
                    print(f"==> {container_name} unhealthy (will fail in 90s if not recovered)", flush=True)
                elif now - unhealthy_since > 90:
                    dump_container_logs(container_name)
                    _dump_docker_ps()
                    raise RuntimeError(f"container {container_name} stuck unhealthy for >90s")
            # status is "starting" or something transient — log periodically
            if now - last_log_time >= 20:
                print(f"==> waiting for {container_name}: status={status} elapsed={int(now - (deadline - timeout_seconds))}s", flush=True)
                last_log_time = now
        except (subprocess.CalledProcessError, OSError) as error:
            last_error = error
        time.sleep(2)
    dump_container_logs(container_name)
    _dump_docker_ps()
    raise RuntimeError(f"timed out waiting for container {container_name}: {last_error}")


def _dump_docker_ps() -> None:
    print("==> docker ps -a:", flush=True)
    subprocess.run(["docker", "ps", "-a"], check=False)


def wait_for_containers(containers: Iterable[str], timeout_seconds: int = 60) -> None:
    for container_name in containers:
        wait_for_container_health(container_name, timeout_seconds=timeout_seconds)


def run_compose_smoke_script(
    script: str,
    service_name: str = "mock-udm",
    env: Mapping[str, str] | None = None,
) -> None:
    arguments = ["run", "--rm", "--no-deps", "-T"]
    for key, value in (env or DEFAULT_SMOKE_RUNNER_ENV).items():
        arguments.extend(["-e", f"{key}={value}"])
    arguments.extend([service_name, "python", script])
    run_compose_command(arguments)