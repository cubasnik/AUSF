from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


AUTOMATION_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(AUTOMATION_ROOT / "src"))

from compose_runtime import compose_down, compose_up, wait_for_containers


PYTHON = sys.executable
HEALTH_CONTAINERS = [
    "mock-udm",
    "mock-nrf",
    "mock-amf",
    "ausf-control-plane",
    "ausf-go",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run only TTL expiration smoke scenario.")
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
        run_host_smoke_script("automation/scripts/smoke_test_http_udm_context_ttl_expired.py")
    finally:
        compose_down(check=False)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())