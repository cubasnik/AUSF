from __future__ import annotations

import json
import os
import time
from http.client import RemoteDisconnected
from urllib import request
from urllib.error import URLError


AUSF_BASE_URL = os.environ.get("AUSF_BASE_URL", "http://127.0.0.1:8080")
CONTROL_PLANE_BASE_URL = os.environ.get("CONTROL_PLANE_BASE_URL", "http://127.0.0.1:8081")
MOCK_UDM_BASE_URL = os.environ.get("MOCK_UDM_BASE_URL", "http://127.0.0.1:8090")
MOCK_NRF_BASE_URL = os.environ.get("MOCK_NRF_BASE_URL", "http://127.0.0.1:8091")
MOCK_AMF_BASE_URL = os.environ.get("MOCK_AMF_BASE_URL", "http://127.0.0.1:8092")

BASIC_HEALTH_ENDPOINTS = [
    f"{MOCK_UDM_BASE_URL}/healthz",
    f"{MOCK_NRF_BASE_URL}/healthz",
    f"{CONTROL_PLANE_BASE_URL}/healthz",
    f"{AUSF_BASE_URL}/healthz",
]


def wait_for_health(url: str, timeout_seconds: int = 60) -> dict:
    deadline = time.time() + timeout_seconds
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            with request.urlopen(url, timeout=5) as response:
                return json.loads(response.read().decode("utf-8"))
        except (URLError, TimeoutError, json.JSONDecodeError, RemoteDisconnected, ConnectionResetError, OSError) as error:
            last_error = error
            time.sleep(1)
    raise RuntimeError(f"timed out waiting for {url}: {last_error}")


def load_amf_notifications() -> list[dict]:
    with request.urlopen(f"{MOCK_AMF_BASE_URL}/notifications", timeout=5) as response:
        return json.loads(response.read().decode("utf-8"))["notifications"]