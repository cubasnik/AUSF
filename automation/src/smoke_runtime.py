from __future__ import annotations

import json
import os
import ssl
import time
from http.client import RemoteDisconnected
from urllib import request
from urllib.error import HTTPError
from urllib.error import URLError
from urllib.parse import urlparse


AUSF_BASE_URL = os.environ.get("AUSF_BASE_URL", "http://127.0.0.1:8080")
CONTROL_PLANE_BASE_URL = os.environ.get("CONTROL_PLANE_BASE_URL", "http://127.0.0.1:8081")
MOCK_UDM_BASE_URL = os.environ.get("MOCK_UDM_BASE_URL", "http://127.0.0.1:8090")
MOCK_NRF_BASE_URL = os.environ.get("MOCK_NRF_BASE_URL", "http://127.0.0.1:8091")
MOCK_AMF_BASE_URL = os.environ.get("MOCK_AMF_BASE_URL", "http://127.0.0.1:8092")
AUSF_CA_CERT_FILE = os.environ.get("AUSF_CA_CERT_FILE")
CONTROL_PLANE_CA_CERT_FILE = os.environ.get("CONTROL_PLANE_CA_CERT_FILE")
EAP_AKA_PRIME_RESPONSE_PREFIX = "EAP-Response/AKA'-Challenge RES*="
EAP_AKA_PRIME_REAUTH_RESPONSE_PREFIX = "EAP-Response/AKA'-Reauthentication "
EAP_AKA_PRIME_FAST_REAUTH_RESPONSE_PREFIX = "EAP-Response/AKA'-Fast-Reauthentication "
EAP_AKA_PRIME_SYNC_FAILURE_RESPONSE_PREFIX = "EAP-Response/AKA'-Synchronization-Failure AUTS="

BASIC_HEALTH_ENDPOINTS = [
    f"{MOCK_UDM_BASE_URL}/healthz",
    f"{MOCK_NRF_BASE_URL}/healthz",
    f"{CONTROL_PLANE_BASE_URL}/healthz",
    f"{AUSF_BASE_URL}/healthz",
]


def fetch_json(url: str) -> dict:
    with request.urlopen(url, timeout=5, context=ssl_context_for_url(url)) as response:
        raw_body = response.read().decode("utf-8")
        return json.loads(raw_body) if raw_body else {}


def wait_for_health(url: str, timeout_seconds: int = 60) -> dict:
    deadline = time.time() + timeout_seconds
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            return fetch_json(url)
        except (URLError, TimeoutError, json.JSONDecodeError, RemoteDisconnected, ConnectionResetError, OSError) as error:
            last_error = error
            time.sleep(1)
    raise RuntimeError(f"timed out waiting for {url}: {last_error}")


def load_amf_notifications() -> list[dict]:
    with request.urlopen(f"{MOCK_AMF_BASE_URL}/notifications", timeout=5) as response:
        return json.loads(response.read().decode("utf-8"))["notifications"]


def load_control_plane_authentication_context(supi: str) -> dict:
    try:
        return fetch_json(f"{CONTROL_PLANE_BASE_URL}/control-plane/v1/auth/{supi}")
    except HTTPError as error:
        raise RuntimeError(f"control-plane authentication context not found for {supi}: {error.code}") from error


def build_eap_aka_prime_response_payload(xres_star: str) -> str:
    return f"{EAP_AKA_PRIME_RESPONSE_PREFIX}{xres_star}"


def build_eap_aka_prime_reauthentication_payload(token: str = "token") -> str:
    return f"{EAP_AKA_PRIME_REAUTH_RESPONSE_PREFIX}{token}"


def build_eap_aka_prime_fast_reauthentication_payload(token: str = "token") -> str:
    return f"{EAP_AKA_PRIME_FAST_REAUTH_RESPONSE_PREFIX}{token}"


def build_eap_aka_prime_synchronization_failure_payload(auts: str = "auts-token") -> str:
    return f"{EAP_AKA_PRIME_SYNC_FAILURE_RESPONSE_PREFIX}{auts}"


def ssl_context_for_url(url: str) -> ssl.SSLContext | None:
    if urlparse(url).scheme != "https":
        return None
    ca_cert_file = ca_cert_file_for_url(url)
    if ca_cert_file:
        return ssl.create_default_context(cafile=ca_cert_file)
    return ssl.create_default_context()


def ca_cert_file_for_url(url: str) -> str | None:
    if url.startswith(AUSF_BASE_URL):
        return AUSF_CA_CERT_FILE
    if url.startswith(CONTROL_PLANE_BASE_URL):
        return CONTROL_PLANE_CA_CERT_FILE
    return None