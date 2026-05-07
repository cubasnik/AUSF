from __future__ import annotations

import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError
from smoke_runtime import AUSF_BASE_URL, load_amf_notifications


def main() -> int:
    if len(sys.argv) != 2:
        raise SystemExit("usage: smoke_test_http_udm_context_ttl_expired_verify.py <authCtxId>")

    auth_ctx_id = sys.argv[1]

    client = AUSFClient(AUSF_BASE_URL)
    print(f"ausf-health-after-restart: {client.health()}")

    try:
        client.get_authentication_context(auth_ctx_id)
    except AUSFError as error:
        assert error.status_code == 404
        assert error.payload["cause"] == "CONTEXT_NOT_FOUND"
        assert error.payload["detail"] == "authentication context not found"
        print(f"ttl-expired-context-error: {error.payload}")
    else:
        raise AssertionError("expected 404 CONTEXT_NOT_FOUND for expired authentication context")

    matching_notifications = [notification for notification in load_amf_notifications() if notification["authCtxId"] == auth_ctx_id]
    assert not matching_notifications
    print("amf-notifications: []")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())