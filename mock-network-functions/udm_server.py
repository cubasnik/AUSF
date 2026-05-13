from __future__ import annotations

import hashlib
import json
import os
import threading
import uuid

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


HOST = os.environ.get("MOCK_UDM_HOST", "0.0.0.0")
PORT = int(os.environ.get("MOCK_UDM_PORT", "8090"))
SUBSCRIBERS_FILE = Path(os.environ.get("MOCK_UDM_SUBSCRIBERS_FILE", "/app/control-plane/data/subscribers.json"))
UNAVAILABLE_SUPIS = {
    value.strip()
    for value in os.environ.get("MOCK_UDM_UNAVAILABLE_SUPIS", "imsi-250010000000503").split(",")
    if value.strip()
}


def load_subscribers() -> dict[str, dict]:
    subscribers = json.loads(SUBSCRIBERS_FILE.read_text(encoding="utf-8"))
    return {subscriber["supi"]: subscriber for subscriber in subscribers}


SUBSCRIBERS = load_subscribers()
SUBSCRIBERS_LOCK = threading.Lock()
LATEST_AUTH_DATA: dict[str, dict] = {}
_auth_events: dict[str, dict] = {}
_AUTH_EVENTS_LOCK = threading.Lock()


def digest_hex(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def generate_auth_data(subscriber: dict, serving_network_name: str, auth_type: str, rand_seed: str = "", auts_seed: str = "") -> dict:
    sequence_material = f"{subscriber['sequenceNumber']}:{rand_seed}:{auts_seed}"
    rand = digest_hex(f"{subscriber['permanentKey']}{subscriber['opc']}{sequence_material}")[:32]
    autn = digest_hex(f"{subscriber['opc']}{serving_network_name}{subscriber['routingIndicator']}{sequence_material}")[:32]
    auts = digest_hex(f"{rand}{subscriber['permanentKey']}{subscriber['opc']}{sequence_material}")[:28]
    xres_star = digest_hex(f"{rand}{autn}{subscriber['permanentKey']}")[:32]
    hxres_star = digest_hex(f"{rand}{xres_star}")[:32]
    kausf = digest_hex(f"{subscriber['permanentKey']}{serving_network_name}")
    eap_challenge = f"EAP-Request/AKA'-Challenge {digest_hex(f'{autn}{xres_star}')[:24]}"

    return {
        "supi": subscriber["supi"],
        "authType": auth_type,
        "servingNetworkName": serving_network_name,
        "rand": rand,
        "autn": autn,
        "auts": auts if auth_type == "5G_AKA" else None,
        "xresStar": xres_star,
        "hxresStar": hxres_star,
        "kausf": kausf,
        "eapChallenge": eap_challenge,
    }


class Handler(BaseHTTPRequestHandler):
    server_version = "MockUdm/1.0"

    def validate_resynchronization_request(self, supi: str, auth_type: str, body: dict) -> tuple[bool, dict]:
        if auth_type != "5G_AKA":
            return True, {}

        if not body.get("rand") and not body.get("auts"):
            return True, {}

        latest_auth_data = LATEST_AUTH_DATA.get(supi)
        if latest_auth_data is None:
            return False, {"detail": "AUTS verification failed", "cause": "AUTHENTICATION_REJECTED"}

        if latest_auth_data.get("rand") != (body.get("rand") or ""):
            return False, {"detail": "AUTS verification failed", "cause": "AUTHENTICATION_REJECTED"}

        if latest_auth_data.get("auts") != (body.get("auts") or ""):
            return False, {"detail": "AUTS verification failed", "cause": "AUTHENTICATION_REJECTED"}

        return True, {}

    def do_GET(self) -> None:
        if self.path == "/healthz":
            self.write_json(200, {"status": "ok", "service": "mock-udm"})
            return
        self.write_json(404, {"detail": "not found"})

    def do_POST(self) -> None:
        prefix = "/nudm-ueau/v1/"
        suffix = "/security-information/generate-auth-data"
        if not self.path.startswith(prefix) or not self.path.endswith(suffix):
            self.write_json(404, {"detail": "not found"})
            return

        supi = self.path[len(prefix):-len(suffix)].strip("/")
        if supi in UNAVAILABLE_SUPIS:
            self.write_json(503, {"detail": f"authentication data temporarily unavailable for {supi}", "cause": "UDM_UNAVAILABLE"})
            return

        content_length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(content_length) or b"{}")
        with SUBSCRIBERS_LOCK:
            subscriber = SUBSCRIBERS.get(supi)
            if subscriber is None:
                self.write_json(404, {"detail": f"subscriber {supi} not found"})
                return

            serving_network_name = body.get("servingNetworkName") or subscriber["servingNetworkName"]
            auth_type = body.get("authType") or subscriber["authMethod"]
            is_valid_resync, error_payload = self.validate_resynchronization_request(supi, auth_type, body)
            if not is_valid_resync:
                self.write_json(400, error_payload)
                return

            response = generate_auth_data(
                subscriber,
                serving_network_name,
                auth_type,
                body.get("rand") or "",
                body.get("auts") or "",
            )
            LATEST_AUTH_DATA[supi] = response
            subscriber["sequenceNumber"] = int(subscriber["sequenceNumber"]) + 1

        auth_event_id = str(uuid.uuid4())
        with _AUTH_EVENTS_LOCK:
            _auth_events[auth_event_id] = {"supi": supi}
        location = f"/nudm-ueau/v1/{supi}/auth-events/{auth_event_id}"
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.send_header("Location", location)
        self.end_headers()
        self.wfile.write(encoded)

    def do_PUT(self) -> None:
        prefix = "/nudm-ueau/v1/"
        auth_events_marker = "/auth-events/"
        if not self.path.startswith(prefix) or auth_events_marker not in self.path:
            self.write_json(404, {"detail": "not found"})
            return

        # /nudm-ueau/v1/{supi}/auth-events/{authEventId}
        rest = self.path[len(prefix):]
        marker_pos = rest.find(auth_events_marker)
        supi = rest[:marker_pos]
        auth_event_id = rest[marker_pos + len(auth_events_marker):].strip("/")

        content_length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(content_length) or b"{}")
        with _AUTH_EVENTS_LOCK:
            if auth_event_id not in _auth_events:
                self.write_json(404, {"detail": f"auth event {auth_event_id} not found"})
                return
            _auth_events[auth_event_id].update(body)
        self.write_json(200, _auth_events.get(auth_event_id, {}))

    def do_DELETE(self) -> None:
        prefix = "/nudm-ueau/v1/"
        auth_events_marker = "/auth-events/"
        if not self.path.startswith(prefix) or auth_events_marker not in self.path:
            self.write_json(404, {"detail": "not found"})
            return

        rest = self.path[len(prefix):]
        marker_pos = rest.find(auth_events_marker)
        auth_event_id = rest[marker_pos + len(auth_events_marker):].strip("/")

        with _AUTH_EVENTS_LOCK:
            _auth_events.pop(auth_event_id, None)
        self.send_response(204)
        self.end_headers()

    def log_message(self, format: str, *args) -> None:
        return

    def write_json(self, status: int, payload: dict) -> None:
        encoded = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)


if __name__ == "__main__":
    ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()