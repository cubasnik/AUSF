from __future__ import annotations

import json
import os

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


HOST = os.environ.get("MOCK_NRF_HOST", "0.0.0.0")
PORT = int(os.environ.get("MOCK_NRF_PORT", "8091"))
UDM_BASE_URL = os.environ.get("MOCK_NRF_UDM_BASE_URL", "http://mock-udm:8090")


class Handler(BaseHTTPRequestHandler):
    server_version = "MockNrf/1.0"

    def do_GET(self) -> None:
        parsed = urlparse(self.path)
        if parsed.path == "/healthz":
            self.write_json(200, {"status": "ok", "service": "mock-nrf"})
            return

        if parsed.path != "/nnrf-disc/v1/nf-instances":
            self.write_json(404, {"detail": "not found"})
            return

        query = parse_qs(parsed.query)
        target_nf_type = (query.get("target-nf-type") or [""])[0]
        requester_nf_type = (query.get("requester-nf-type") or [""])[0]
        if target_nf_type != "UDM" or requester_nf_type != "AUSF":
            self.write_json(400, {"detail": "unsupported discovery query"})
            return

        self.write_json(200, {
            "nfInstances": [
                {
                    "nfInstanceId": "mock-udm-1",
                    "nfType": "UDM",
                    "services": [
                        {
                            "serviceName": "nudm-ueau",
                            "apiPrefix": UDM_BASE_URL,
                        }
                    ],
                }
            ]
        })

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