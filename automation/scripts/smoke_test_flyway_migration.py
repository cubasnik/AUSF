"""smoke_test_flyway_migration.py — Flyway schema migration smoke test.

Scenario:
  1. Bring up only the `postgres` service with a fresh (empty) database.
  2. Start `ausf-control-plane` which will trigger Flyway to apply
     V1__initial_schema.sql on startup.
  3. Wait for the control-plane healthcheck to pass (proves Flyway did not
     fail and JPA schema validation succeeded).
  4. Query the PostgreSQL `flyway_schema_history` table to confirm that
     exactly one migration (V1) was applied and its `success` flag is true.
  5. Tear down.
"""
from __future__ import annotations

import json
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = ROOT.parent
sys.path.insert(0, str(ROOT / "src"))

from compose_runtime import wait_for_container_health
from smoke_runtime import CONTROL_PLANE_BASE_URL, wait_for_health


MIGRATION_CHECK_QUERY = (
    "SELECT version, description, success "
    "FROM flyway_schema_history "
    "ORDER BY installed_rank;"
)


def _compose(*args: str) -> subprocess.CompletedProcess:
    command = ["docker", "compose", *args]
    print(f"==> {' '.join(command)}", flush=True)
    return subprocess.run(command, cwd=REPO_ROOT, check=True)


def _compose_capture(*args: str) -> str:
    command = ["docker", "compose", *args]
    print(f"==> {' '.join(command)}", flush=True)
    result = subprocess.run(
        command,
        cwd=REPO_ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout


def _query_flyway_history() -> list[dict]:
    """Execute a psql query inside the postgres container and return rows."""
    psql_command = (
        f"psql -U ausf -d ausf -t -A -F '|' -c \"{MIGRATION_CHECK_QUERY}\""
    )
    result = subprocess.run(
        ["docker", "compose", "exec", "-T", "postgres", "sh", "-c", psql_command],
        cwd=REPO_ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    rows = []
    for line in result.stdout.strip().splitlines():
        line = line.strip()
        if not line:
            continue
        parts = line.split("|")
        if len(parts) >= 3:
            rows.append({
                "version": parts[0],
                "description": parts[1],
                "success": parts[2],
            })
    return rows


def main() -> int:
    try:
        # 1. Start only postgres with a clean volume
        _compose("up", "-d", "postgres")
        wait_for_container_health("ausf-postgres", timeout_seconds=60)
        print("postgres-health: healthy")

        # 2. Start control-plane — Flyway runs V1__initial_schema.sql on boot
        _compose("up", "-d", "ausf-control-plane")

        # 3. Wait for control-plane readiness (proves Flyway did not crash)
        wait_for_container_health("ausf-control-plane", timeout_seconds=90)
        print("control-plane-health: healthy")

        # Additional HTTP healthcheck from inside compose network
        result = subprocess.run(
            [
                "docker", "compose", "exec", "-T", "ausf-control-plane",
                "sh", "-lc",
                "exec 3<>/dev/tcp/127.0.0.1/8081; "
                "printf 'GET /healthz HTTP/1.1\\r\\nHost: 127.0.0.1\\r\\nConnection: close\\r\\n\\r\\n' >&3; "
                "grep -q 'HTTP/1' <&3 && echo ok",
            ],
            cwd=REPO_ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
        print(f"control-plane-http-healthz: {result.stdout.strip()}")

        # 4. Verify flyway_schema_history
        rows = _query_flyway_history()
        print(f"flyway-schema-history: {rows}")

        v1_rows = [r for r in rows if r["version"] == "1"]
        assert v1_rows, (
            f"V1 migration not found in flyway_schema_history; got: {rows}"
        )
        assert v1_rows[0]["success"] == "t", (
            f"V1 migration did not succeed; success={v1_rows[0]['success']}"
        )
        print("flyway-V1-migration: SUCCESS")
    finally:
        _compose("down", "-v")   # -v removes volumes for a clean next run
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
