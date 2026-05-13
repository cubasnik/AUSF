from __future__ import annotations

import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient
from smoke_runtime import AUSF_BASE_URL, load_amf_notifications


def main() -> int:
    if len(sys.argv) != 3:
        raise SystemExit("usage: smoke_test_http_udm_context_survives_restart_verify.py <authCtxId> <expectedResStar>")

    auth_ctx_id = sys.argv[1]
    expected_res_star = sys.argv[2]

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health-after-restart: {client.health()}")

    context = client.get_authentication_context(auth_ctx_id)
    assert context["authCtxId"] == auth_ctx_id
    assert context["status"] == "CHALLENGE_SENT"
    print(f"context-after-restart: {context}")

    confirmed = client.confirm_authentication(auth_ctx_id, expected_res_star)
    print(f"confirmed-after-restart: {confirmed}")

    matching_notification = next(notification for notification in load_amf_notifications() if notification["authCtxId"] == auth_ctx_id)
    assert matching_notification["authResult"] == "SUCCESS"
    print(f"amf-notification: {matching_notification}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())