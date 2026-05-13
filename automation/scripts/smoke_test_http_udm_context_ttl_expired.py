from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import restart_compose_service, wait_for_container_health
from staged_compose_smoke import read_prepared_state, run_compose_network_script, wait_for_compose_services


STORE_PATH = "/tmp/ausf-auth-contexts.json"


def read_auth_context_store() -> dict:
    result = subprocess.run(
        ["docker", "compose", "exec", "-T", "ausf-go", "sh", "-lc", f"cat {STORE_PATH}"],
        cwd=ROOT.parent,
        check=True,
        capture_output=True,
        text=True,
    )
    return json.loads(result.stdout)


def write_auth_context_store(payload: dict) -> None:
    raw = json.dumps(payload, ensure_ascii=True)
    subprocess.run(
        ["docker", "compose", "exec", "-T", "ausf-go", "sh", "-lc", f"cat > {STORE_PATH}"],
        cwd=ROOT.parent,
        input=raw,
        text=True,
        check=True,
    )


def main() -> int:
    wait_for_compose_services(["mock-nrf", "mock-udm", "mock-amf", "ausf-go"])

    prepared_output = run_compose_network_script(
        "automation/scripts/smoke_test_http_udm_context_ttl_expired_prepare.py",
    )
    prepared_state = read_prepared_state(prepared_output)

    store = read_auth_context_store()
    contexts = store.get("contexts", {})
    context = contexts.get(prepared_state["authCtxId"])
    if context is None:
        raise AssertionError(f"auth context {prepared_state['authCtxId']} not found in persisted store")

    context["createdAt"] = "2000-01-01T00:00:00Z"
    write_auth_context_store(store)

    restart_compose_service("ausf-go")
    wait_for_container_health("ausf-go")

    run_compose_network_script(
        "automation/scripts/smoke_test_http_udm_context_ttl_expired_verify.py",
        prepared_state["authCtxId"],
    )

    store_after = read_auth_context_store()
    assert prepared_state["authCtxId"] not in store_after.get("contexts", {})
    print("expired-context-cleanup: removed from persisted auth context store")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())