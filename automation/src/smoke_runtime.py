from __future__ import annotations

import base64
import hashlib
import hmac as hmac_module
import json
import os
import ssl
import struct
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

# EAP-AKA' constants (RFC 4187 / RFC 5448)
_EAP_CODE_REQUEST  = 0x01
_EAP_CODE_RESPONSE = 0x02
_EAP_CODE_FAILURE  = 0x04
_EAP_TYPE_AKA_PRIME = 50  # 0x32
_SUBTYPE_AKA_CHALLENGE        = 0x01
_SUBTYPE_AKA_SYNC_FAILURE     = 0x04
_SUBTYPE_AKA_REAUTHENTICATION = 0x0D
_AT_RAND      = 0x01
_AT_AUTN      = 0x02
_AT_RES       = 0x03
_AT_AUTS      = 0x10
_AT_MAC       = 0x0B
_AT_KDF       = 0x18
_AT_KDF_INPUT = 0x17
_AT_NONCE_S   = 0x15

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


# ---------------------------------------------------------------------------
# Binary EAP-AKA' helpers (RFC 4187 / RFC 5448)
# ---------------------------------------------------------------------------

def _b64url_decode(s: str) -> bytes:
    """Decode base64url string with or without padding."""
    rem = len(s) % 4
    padded = s + "=" * (4 - rem) if rem else s
    return base64.urlsafe_b64decode(padded)


def _b64url_encode(data: bytes) -> str:
    """Encode bytes as base64url without padding."""
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def derive_kaut_prime(kausf_hex: str) -> bytes:
    """Derive K_aut' = HMAC-SHA-256(KAUSF, b\"K_aut'\") — same formula as Java side."""
    return hmac_module.new(bytes.fromhex(kausf_hex), b"K_aut'", hashlib.sha256).digest()


def _compute_eap_mac(kaut_prime: bytes, packet: bytes) -> bytes:
    """HMAC-SHA-256(K_aut', packet) first 16 bytes."""
    return hmac_module.new(kaut_prime, packet, hashlib.sha256).digest()[:16]


def _find_mac_offset(packet: bytes) -> int:
    """Return the byte offset of the 16-byte MAC value inside AT_MAC, or -1."""
    off = 8
    while off + 2 <= len(packet):
        at_type = packet[off]
        at_len_words = packet[off + 1]
        at_bytes = at_len_words * 4
        if at_bytes == 0 or off + at_bytes > len(packet):
            break
        if at_type == _AT_MAC and at_bytes >= 20:
            return off + 4  # MAC starts after type(1)+len(1)+reserved(2)
        off += at_bytes
    return -1


def _fill_mac(kaut_prime: bytes, packet: bytearray) -> None:
    """Compute and fill AT_MAC into packet in-place (MAC field must already be zero)."""
    mac_off = _find_mac_offset(bytes(packet))
    if mac_off < 0:
        return
    mac = _compute_eap_mac(kaut_prime, bytes(packet))
    packet[mac_off:mac_off + 16] = mac


def extract_eap_identifier(base64url_payload: str) -> int:
    """Extract the EAP identifier byte (byte[1]) from a base64url EAP packet."""
    packet = _b64url_decode(base64url_payload)
    return packet[1] if len(packet) >= 2 else 0


def get_eap_context(supi: str, eap_challenge_payload: str) -> dict:
    """
    Return a dict with keys: identifier (int), kaut_prime (bytes), xres_star (str).
    Fetches KAUSF and XRES* from the control-plane debug endpoint.
    """
    ctx = load_control_plane_authentication_context(supi)
    kausf = ctx["kausf"]
    xres_star = ctx["xresStar"]
    kaut_prime = derive_kaut_prime(kausf)
    identifier = extract_eap_identifier(eap_challenge_payload)
    return {"identifier": identifier, "kaut_prime": kaut_prime, "xres_star": xres_star}


def is_eap_failure(base64url_payload: str) -> bool:
    """Return True if the packet is an EAP-Failure (Code=4)."""
    try:
        packet = _b64url_decode(base64url_payload)
        return len(packet) >= 1 and packet[0] == _EAP_CODE_FAILURE
    except Exception:
        return False


def is_eap_challenge(base64url_payload: str) -> bool:
    """Return True if the packet is an EAP-Request/AKA'-Challenge."""
    try:
        packet = _b64url_decode(base64url_payload)
        return (
            len(packet) >= 6
            and packet[0] == _EAP_CODE_REQUEST
            and packet[4] == _EAP_TYPE_AKA_PRIME
            and packet[5] == _SUBTYPE_AKA_CHALLENGE
        )
    except Exception:
        return False


def build_eap_aka_prime_response_payload(xres_star: str, eap_ctx: dict) -> str:
    """
    Build a binary EAP-Response/AKA'-Challenge with AT_RES + AT_MAC.

    :param xres_star: hex string of the RES* value to include
    :param eap_ctx: dict with 'identifier' (int) and 'kaut_prime' (bytes)
    """
    identifier = eap_ctx["identifier"]
    kaut_prime = eap_ctx["kaut_prime"]
    res_bytes = bytes.fromhex(xres_star)
    res_len = len(res_bytes)
    res_padded_len = (res_len + 3) & ~3
    # AT_RES: type(1)+len(1)+actual_bits(2)+res_padded
    at_res_total = 2 + 2 + res_padded_len  # but len field = at_res_total/4
    at_res_words = at_res_total // 4
    at_res_total = at_res_words * 4  # normalise
    packet_len = 8 + at_res_total + 20
    pkt = bytearray(packet_len)
    off = 0
    pkt[off] = _EAP_CODE_RESPONSE; off += 1
    pkt[off] = identifier & 0xFF; off += 1
    pkt[off] = (packet_len >> 8) & 0xFF; off += 1
    pkt[off] = packet_len & 0xFF; off += 1
    pkt[off] = _EAP_TYPE_AKA_PRIME; off += 1
    pkt[off] = _SUBTYPE_AKA_CHALLENGE; off += 1
    off += 2  # reserved
    pkt[off] = _AT_RES; off += 1
    pkt[off] = at_res_words; off += 1
    actual_bits = res_len * 8
    pkt[off] = (actual_bits >> 8) & 0xFF; off += 1
    pkt[off] = actual_bits & 0xFF; off += 1
    pkt[off:off + res_len] = res_bytes
    off += at_res_total - 4  # advance past padded res
    pkt[off] = _AT_MAC; off += 1
    pkt[off] = 5; off += 1  # 5 * 4 = 20 bytes
    # reserved(2) + MAC(16) already zero
    _fill_mac(kaut_prime, pkt)
    return _b64url_encode(bytes(pkt))


def build_eap_aka_prime_reauthentication_payload(eap_ctx: dict, token: str = "token") -> str:
    """
    Build a binary EAP-Response/AKA'-Reauthentication (regular, no AT_NONCE_S).
    The ``token`` parameter is kept for backward-compat but is ignored.
    """
    return _build_reauth_payload(eap_ctx, fast=False)


def build_eap_aka_prime_fast_reauthentication_payload(eap_ctx: dict, token: str = "token") -> str:
    """
    Build a binary EAP-Response/AKA'-Reauthentication with AT_NONCE_S (fast reauth).
    """
    return _build_reauth_payload(eap_ctx, fast=True)


def _build_reauth_payload(eap_ctx: dict, fast: bool) -> str:
    identifier = eap_ctx["identifier"]
    kaut_prime = eap_ctx["kaut_prime"]
    nonce_s_bytes = 20 if fast else 0  # AT_NONCE_S: type+len+reserved+16_nonce
    packet_len = 8 + nonce_s_bytes + 20
    pkt = bytearray(packet_len)
    off = 0
    pkt[off] = _EAP_CODE_RESPONSE; off += 1
    pkt[off] = identifier & 0xFF; off += 1
    pkt[off] = (packet_len >> 8) & 0xFF; off += 1
    pkt[off] = packet_len & 0xFF; off += 1
    pkt[off] = _EAP_TYPE_AKA_PRIME; off += 1
    pkt[off] = _SUBTYPE_AKA_REAUTHENTICATION; off += 1
    off += 2  # reserved
    if fast:
        pkt[off] = _AT_NONCE_S; off += 1
        pkt[off] = 5; off += 1          # 5*4=20 bytes
        off += 2                         # reserved
        import os as _os
        nonce = _os.urandom(16)
        pkt[off:off + 16] = nonce; off += 16
    pkt[off] = _AT_MAC; off += 1
    pkt[off] = 5; off += 1
    # reserved(2) + MAC(16) already zero
    _fill_mac(kaut_prime, pkt)
    return _b64url_encode(bytes(pkt))


def build_eap_aka_prime_synchronization_failure_payload(auts: str, eap_ctx: dict) -> str:
    """
    Build a binary EAP-Response/AKA'-Synchronization-Failure with AT_AUTS + AT_MAC.

    :param auts: hex string of the 14-byte AUTS value
    :param eap_ctx: dict with 'identifier' (int) and 'kaut_prime' (bytes)
    """
    identifier = eap_ctx["identifier"]
    kaut_prime = eap_ctx["kaut_prime"]
    auts_bytes = bytes.fromhex(auts)
    auts_len = min(len(auts_bytes), 14)
    # AT_AUTS: type(1)+len(1)+auts(14) = 16 bytes; len field = 4
    packet_len = 8 + 16 + 20
    pkt = bytearray(packet_len)
    off = 0
    pkt[off] = _EAP_CODE_RESPONSE; off += 1
    pkt[off] = identifier & 0xFF; off += 1
    pkt[off] = (packet_len >> 8) & 0xFF; off += 1
    pkt[off] = packet_len & 0xFF; off += 1
    pkt[off] = _EAP_TYPE_AKA_PRIME; off += 1
    pkt[off] = _SUBTYPE_AKA_SYNC_FAILURE; off += 1
    off += 2  # reserved
    pkt[off] = _AT_AUTS; off += 1
    pkt[off] = 4; off += 1             # 4*4=16 bytes total
    pkt[off:off + auts_len] = auts_bytes[:auts_len]; off += 14
    pkt[off] = _AT_MAC; off += 1
    pkt[off] = 5; off += 1
    # reserved(2) + MAC(16) already zero
    _fill_mac(kaut_prime, pkt)
    return _b64url_encode(bytes(pkt))


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