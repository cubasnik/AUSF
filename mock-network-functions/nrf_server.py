from __future__ import annotations

import json
import os
import threading
import urllib.request
import uuid

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


HOST = os.environ.get("MOCK_NRF_HOST", "0.0.0.0")
PORT = int(os.environ.get("MOCK_NRF_PORT", "8091"))
UDM_BASE_URL = os.environ.get("MOCK_NRF_UDM_BASE_URL", "http://mock-udm:8090")

# In-memory registry of NF instances and subscriptions
_lock = threading.Lock()
_nf_instances: dict[str, dict] = {}
_subscriptions: dict[str, dict] = {}       # NFm subscriptions
_disc_subscriptions: dict[str, dict] = {}  # NFDiscovery subscriptions


class Handler(BaseHTTPRequestHandler):
    server_version = "MockNrf/1.0"

    def do_GET(self) -> None:
        parsed = urlparse(self.path)

        if parsed.path == "/healthz":
            self.write_json(200, {"status": "ok", "service": "mock-nrf"})
            return

        if parsed.path != "/nnrf-disc/v1/nf-instances":
            # Check: GET /nnrf-nfm/v1/nf-instances/{id}
            prefix = "/nnrf-nfm/v1/nf-instances/"
            if parsed.path.startswith(prefix):
                nf_id = parsed.path[len(prefix):]
                with _lock:
                    profile = _nf_instances.get(nf_id)
                if profile is not None:
                    self.write_json(200, profile)
                else:
                    self.write_json(404, {"detail": "NF instance not found"})
                return

            self.write_json(404, {"detail": "not found"})
            return

        query = parse_qs(parsed.query)
        target_nf_type = (query.get("target-nf-type") or [""])[0]

        with _lock:
            matching = [
                p for p in _nf_instances.values()
                if not target_nf_type or p.get("nfType") == target_nf_type
            ]

        # Fall back to a static UDM entry so smoke tests work before any PUT
        if not matching and target_nf_type == "UDM":
            matching = [{
                "nfInstanceId": "mock-udm-1",
                "nfType": "UDM",
                "nfStatus": "REGISTERED",
                "services": [{"serviceName": "nudm-ueau", "apiPrefix": UDM_BASE_URL}],
            }]

        self.write_json(200, {"nfInstances": matching})

    def do_PUT(self) -> None:
        parsed = urlparse(self.path)
        prefix = "/nnrf-nfm/v1/nf-instances/"
        if not parsed.path.startswith(prefix):
            self.write_json(404, {"detail": "not found"})
            return
        nf_id = parsed.path[len(prefix):]
        body = self._read_body()
        with _lock:
            is_new = nf_id not in _nf_instances
            _nf_instances[nf_id] = body
        status = 201 if is_new else 200
        self.send_response(status)
        self.send_header("Content-Length", "0")
        self.end_headers()
        if is_new:
            _deliver_notifications("NF_REGISTERED", nf_id)
            _deliver_disc_notifications(body)

    def do_PATCH(self) -> None:
        parsed = urlparse(self.path)
        prefix = "/nnrf-nfm/v1/nf-instances/"
        if not parsed.path.startswith(prefix):
            self.write_json(404, {"detail": "not found"})
            return
        self._read_body()  # consume body
        self.send_response(204)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_POST(self) -> None:
        parsed = urlparse(self.path)
        nfm_subs_path = "/nnrf-nfm/v1/subscriptions"
        disc_subs_path = "/nnrf-disc/v1/subscriptions"

        if parsed.path == nfm_subs_path:
            body = self._read_body()
            subs_id = str(uuid.uuid4())
            with _lock:
                _subscriptions[subs_id] = body
            location = (f"http://{self.server.server_address[0]}:{self.server.server_address[1]}"
                        f"{nfm_subs_path}/{subs_id}")
            self._respond_created(location, {"subsId": subs_id})

        elif parsed.path == disc_subs_path:
            body = self._read_body()
            subs_id = str(uuid.uuid4())
            with _lock:
                _disc_subscriptions[subs_id] = body
            location = (f"http://{self.server.server_address[0]}:{self.server.server_address[1]}"
                        f"{disc_subs_path}/{subs_id}")
            self._respond_created(location, {"subsId": subs_id})

        else:
            self.write_json(404, {"detail": "not found"})

    def _respond_created(self, location: str, payload: dict) -> None:
        encoded = json.dumps(payload).encode("utf-8")
        self.send_response(201)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.send_header("Location", location)
        self.end_headers()
        self.wfile.write(encoded)

    def do_DELETE(self) -> None:
        parsed = urlparse(self.path)
        nfm_prefix = "/nnrf-nfm/v1/nf-instances/"
        nfm_subs_prefix = "/nnrf-nfm/v1/subscriptions/"
        disc_subs_prefix = "/nnrf-disc/v1/subscriptions/"
        if parsed.path.startswith(nfm_prefix):
            nf_id = parsed.path[len(nfm_prefix):]
            with _lock:
                existed = _nf_instances.pop(nf_id, None) is not None
            if existed:
                _deliver_notifications("NF_DEREGISTERED", nf_id)
            self.send_response(204)
            self.send_header("Content-Length", "0")
            self.end_headers()
        elif parsed.path.startswith(nfm_subs_prefix):
            subs_id = parsed.path[len(nfm_subs_prefix):]
            with _lock:
                _subscriptions.pop(subs_id, None)
            self.send_response(204)
            self.send_header("Content-Length", "0")
            self.end_headers()
        elif parsed.path.startswith(disc_subs_prefix):
            subs_id = parsed.path[len(disc_subs_prefix):]
            with _lock:
                _disc_subscriptions.pop(subs_id, None)
            self.send_response(204)
            self.send_header("Content-Length", "0")
            self.end_headers()
        else:
            self.write_json(404, {"detail": "not found"})

    def log_message(self, format: str, *args) -> None:
        return

    def _read_body(self) -> dict:
        length = int(self.headers.get("Content-Length", 0))
        if length == 0:
            return {}
        raw = self.rfile.read(length)
        try:
            return json.loads(raw)
        except Exception:
            return {}

    def write_json(self, status: int, payload: dict) -> None:
        encoded = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)


def _deliver_disc_notifications(nf_profile: dict) -> None:
    """Deliver NF-discovery notifications to matching disc subscribers."""
    nf_type = nf_profile.get("nfType", "")
    with _lock:
        subscribers = list(_disc_subscriptions.values())

    def _send(sub: dict) -> None:
        uri = sub.get("callbackUri")
        if not uri:
            return
        nf_type_condition = sub.get("nfTypeCondition", {})
        interested_types = nf_type_condition.get("nfTypes", [])
        if interested_types and nf_type not in interested_types:
            return
        payload = json.dumps({
            "event": "NF_REGISTERED",
            "nfProfile": nf_profile,
        }).encode("utf-8")
        try:
            req = urllib.request.Request(uri, data=payload, method="POST",
                                         headers={"Content-Type": "application/json",
                                                  "Content-Length": str(len(payload))})
            urllib.request.urlopen(req, timeout=3)
        except Exception:
            pass  # best-effort delivery

    for sub in subscribers:
        threading.Thread(target=_send, args=(sub,), daemon=True).start()


def _deliver_notifications(event: str, nf_instance_id: str) -> None:
    """Deliver NF-status notifications to all subscribers in a background thread."""
    with _lock:
        subscribers = list(_subscriptions.values())

    def _send(sub: dict) -> None:
        uri = sub.get("nfStatusNotificationUri")
        if not uri:
            return
        req_events = sub.get("reqNotifEvents", [])
        if req_events and event not in req_events:
            return
        payload = json.dumps({
            "event": event,
            "nfInstanceUri": f"/nnrf-nfm/v1/nf-instances/{nf_instance_id}",
        }).encode("utf-8")
        try:
            req = urllib.request.Request(uri, data=payload, method="POST",
                                         headers={"Content-Type": "application/json",
                                                  "Content-Length": str(len(payload))})
            urllib.request.urlopen(req, timeout=3)
        except Exception:
            pass  # best-effort delivery

    for sub in subscribers:
        threading.Thread(target=_send, args=(sub,), daemon=True).start()


if __name__ == "__main__":
    ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()
