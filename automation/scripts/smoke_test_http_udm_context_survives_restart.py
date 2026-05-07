from __future__ import annotations

import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import restart_compose_service, wait_for_container_health
from staged_compose_smoke import read_prepared_state, run_compose_network_script, wait_for_compose_services


def main() -> int:
    wait_for_compose_services(["mock-nrf", "mock-udm", "mock-amf", "ausf-go"])

    prepared_output = run_compose_network_script(
        "automation/scripts/smoke_test_http_udm_context_survives_restart_prepare.py",
    )
    prepared_state = read_prepared_state(prepared_output)

    restart_compose_service("ausf-go")
    wait_for_container_health("ausf-go")

    run_compose_network_script(
        "automation/scripts/smoke_test_http_udm_context_survives_restart_verify.py",
        prepared_state["authCtxId"],
        prepared_state["expectedResStar"],
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())