from __future__ import annotations

import hashlib
import json
import os

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


def digest_hex(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def generate_auth_data(subscriber: dict, serving_network_name: str, auth_type: str) -> dict:
    rand = digest_hex(f"{subscriber['permanentKey']}{subscriber['opc']}{subscriber['sequenceNumber']}")[:32]
    autn = digest_hex(f"{subscriber['opc']}{serving_network_name}{subscriber['routingIndicator']}")[:32]
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
        "xresStar": xres_star,
        "hxresStar": hxres_star,
        "kausf": kausf,
        "eapChallenge": eap_challenge,
    }


class Handler(BaseHTTPRequestHandler):
    server_version = "MockUdm/1.0"

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

        subscribers = load_subscribers()
        subscriber = subscribers.get(supi)
        if subscriber is None:
            self.write_json(404, {"detail": f"subscriber {supi} not found"})
            return

        content_length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(content_length) or b"{}")
        serving_network_name = body.get("servingNetworkName") or subscriber["servingNetworkName"]
        auth_type = body.get("authType") or subscriber["authMethod"]
        self.write_json(200, generate_auth_data(subscriber, serving_network_name, auth_type))

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