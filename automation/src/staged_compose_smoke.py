from __future__ import annotations

import json
import sys
from collections.abc import Iterable

from compose_runtime import DEFAULT_SMOKE_RUNNER_ENV, run_compose_subprocess, wait_for_container_health


def wait_for_compose_services(container_names: Iterable[str]) -> None:
    for container_name in container_names:
        wait_for_container_health(container_name)


def run_compose_network_script(script: str, *script_args: str) -> str:
    arguments = ["run", "--rm", "--no-deps", "-T"]
    for key, value in DEFAULT_SMOKE_RUNNER_ENV.items():
        arguments.extend(["-e", f"{key}={value}"])
    arguments.extend(["mock-udm", "python", script, *script_args])
    result = run_compose_subprocess(arguments, capture_output=True, text=True)
    if result.stdout:
        print(result.stdout, end="")
    if result.stderr:
        print(result.stderr, end="", file=sys.stderr)
    return result.stdout


def read_prepared_state(output: str) -> dict:
    for line in reversed(output.splitlines()):
        if line.startswith("prepared-state: "):
            return json.loads(line.removeprefix("prepared-state: "))
    raise AssertionError("prepare script did not emit prepared-state")