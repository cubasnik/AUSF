from __future__ import annotations

import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from smoke_client import AUSFClient, AUSFError


class _Handler(BaseHTTPRequestHandler):
    last_create_payload: dict | None = None
    last_confirm_path: str | None = None

    def do_GET(self) -> None:
        if self.path == "/healthz":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "ok"}).encode("utf-8"))
            return
        if self.path == "/nausf-auth/v1/ue-authentications/auth-1":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"authCtxId": "auth-1", "status": "CHALLENGE_SENT"}).encode("utf-8"))
            return
        if self.path == "/nausf-auth/v1/ue-authentications/auth-eap":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"authCtxId": "auth-eap", "status": "CHALLENGE_SENT"}).encode("utf-8"))
            return

        self.send_response(404)
        self.end_headers()

    def do_DELETE(self) -> None:
        if self.path != "/nausf-auth/v1/ue-authentications/auth-1":
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(204)
        self.end_headers()

    def do_POST(self) -> None:
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        payload = json.loads(body.decode("utf-8"))
        if self.path == "/nausf-auth/v1/ue-authentications":
            _Handler.last_create_payload = payload
            if payload["supiOrSuci"] == "bad-request":
                self.send_response(400)
                self.send_header("Content-Type", "application/problem+json")
                self.end_headers()
                self.wfile.write(json.dumps({"title": "Invalid request", "status": 400, "detail": "bad input"}).encode("utf-8"))
                return
            if payload["authType"] == "EAP_AKA_PRIME":
                response = {
                    "authCtxId": "auth-eap",
                    "supi": payload["supiOrSuci"],
                    "servingNetworkName": payload["servingNetworkName"],
                    "authType": payload["authType"],
                    "eapSession": {"method": "EAP-AKA'", "payload": "EAP-Request/AKA'-Challenge token", "sessionId": "auth-eap"},
                    "_links": {"eap-session": {"href": "/nausf-auth/v1/ue-authentications/auth-eap/eap-session"}},
                }
                self.send_response(201)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps(response).encode("utf-8"))
                return
            response = {
                "authCtxId": "auth-1",
                "supi": payload["supiOrSuci"],
                "servingNetworkName": payload["servingNetworkName"],
                "authType": payload["authType"],
                "5gAuthData": {"rand": "a" * 32, "autn": "b" * 32, "hxresStar": "c" * 32},
                "_links": {"5g-aka": {"href": "/nausf-auth/v1/ue-authentications/auth-1/5g-aka-confirmation"}},
            }
            self.send_response(201)
        else:
            _Handler.last_confirm_path = self.path
            response = {"authCtxId": "auth-1", "authResult": "SUCCESS", "kseaf": payload.get("resStar") or payload.get("eapPayload")}
            self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(response).encode("utf-8"))

    def log_message(self, format: str, *args) -> None:
        return


class AUSFClientTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = HTTPServer(("127.0.0.1", 0), _Handler)
        cls.base_url = f"http://127.0.0.1:{cls.server.server_address[1]}"
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=1)

    def test_client_flow(self) -> None:
        client = AUSFClient(self.base_url)

        health = client.health()
        challenge = client.initiate_authentication("imsi-250010000000001", "5G:mnc001.mcc001.3gppnetwork.org")
        context = client.get_authentication_context("auth-1")
        confirmed = client.confirm_authentication("auth-1", "feedface")
        client.delete_authentication_context("auth-1")

        self.assertEqual("ok", health["status"])
        self.assertEqual("auth-1", challenge["authCtxId"])
        self.assertEqual("CHALLENGE_SENT", context["status"])
        self.assertEqual("SUCCESS", confirmed["authResult"])

    def test_eap_aka_prime_flow(self) -> None:
        client = AUSFClient(self.base_url)

        challenge = client.initiate_authentication(
            "imsi-250010000000002",
            "5G:mnc001.mcc001.3gppnetwork.org",
            auth_type="EAP_AKA_PRIME",
        )
        confirmed = client.confirm_eap_authentication("auth-eap", "EAP-Response/AKA'-Challenge token")

        self.assertEqual("EAP_AKA_PRIME", challenge["authType"])
        self.assertEqual("EAP-AKA'", challenge["eapSession"]["method"])
        self.assertEqual("/nausf-auth/v1/ue-authentications/auth-eap/eap-session", _Handler.last_confirm_path)
        self.assertEqual("SUCCESS", confirmed["authResult"])

    def test_problem_details_are_raised(self) -> None:
        client = AUSFClient(self.base_url)

        with self.assertRaises(AUSFError) as error:
            client.initiate_authentication("bad-request", "5G:mnc001.mcc001.3gppnetwork.org")

        self.assertEqual(400, error.exception.status_code)
        self.assertEqual("Invalid request", error.exception.payload["title"])

    def test_notification_uri_is_sent_when_provided(self) -> None:
        client = AUSFClient(self.base_url)

        client.initiate_authentication(
            "imsi-250010000000001",
            "5G:mnc001.mcc001.3gppnetwork.org",
            notification_uri="http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
        )

        self.assertEqual(
            "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify",
            _Handler.last_create_payload["notificationUri"],
        )


if __name__ == "__main__":
    unittest.main()