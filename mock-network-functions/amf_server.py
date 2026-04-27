from __future__ import annotations

import json
import os
import threading

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


HOST = os.environ.get("MOCK_AMF_HOST", "0.0.0.0")
PORT = int(os.environ.get("MOCK_AMF_PORT", "8092"))
NOTIFICATIONS: list[dict] = []
NOTIFICATIONS_LOCK = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    server_version = "MockAmf/1.0"

    def do_GET(self) -> None:
        if self.path == "/healthz":
            self.write_json(200, {"status": "ok", "service": "mock-amf"})
            return
        if self.path == "/notifications":
            with NOTIFICATIONS_LOCK:
                self.write_json(200, {"notifications": NOTIFICATIONS})
            return
        self.write_json(404, {"detail": "not found"})

    def do_POST(self) -> None:
        prefix = "/namf-comm/v1/ue-authentications/"
        suffix = "/status-notify"
        if not self.path.startswith(prefix) or not self.path.endswith(suffix):
            self.write_json(404, {"detail": "not found"})
            return

        content_length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(content_length) or b"{}")
        payload["_requestPath"] = self.path
        with NOTIFICATIONS_LOCK:
            NOTIFICATIONS.append(payload)
        self.write_json(204, {})

    def log_message(self, format: str, *args) -> None:
        return

    def write_json(self, status: int, payload: dict) -> None:
        encoded = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        if encoded:
            self.wfile.write(encoded)


if __name__ == "__main__":
    ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()