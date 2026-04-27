from __future__ import annotations

import json
from dataclasses import dataclass
from urllib import request
from urllib.error import HTTPError


class AUSFError(Exception):
    def __init__(self, status_code: int, payload: dict):
        self.status_code = status_code
        self.payload = payload
        super().__init__(payload.get("detail") or payload.get("title") or f"HTTP {status_code}")


@dataclass
class AUSFClient:
    base_url: str

    def health(self) -> dict:
        return self._call("GET", "/healthz")

    def initiate_authentication(
        self,
        supi: str,
        serving_network_name: str,
        auth_type: str = "5G_AKA",
        notification_uri: str | None = None,
    ) -> dict:
        payload = {
            "supiOrSuci": supi,
            "servingNetworkName": serving_network_name,
            "authType": auth_type,
        }
        if notification_uri:
            payload["notificationUri"] = notification_uri
        return self._call(
            "POST",
            "/nausf-auth/v1/ue-authentications",
            payload,
        )

    def confirm_authentication(self, auth_ctx_id: str, res_star: str) -> dict:
        return self._call(
            "POST",
            f"/nausf-auth/v1/ue-authentications/{auth_ctx_id}/5g-aka-confirmation",
            {"resStar": res_star},
        )

    def confirm_eap_authentication(self, auth_ctx_id: str, eap_payload: str) -> dict:
        return self._call(
            "POST",
            f"/nausf-auth/v1/ue-authentications/{auth_ctx_id}/5g-aka-confirmation",
            {"eapPayload": eap_payload},
        )

    def get_authentication_context(self, auth_ctx_id: str) -> dict:
        return self._call("GET", f"/nausf-auth/v1/ue-authentications/{auth_ctx_id}")

    def delete_authentication_context(self, auth_ctx_id: str) -> None:
        self._call("DELETE", f"/nausf-auth/v1/ue-authentications/{auth_ctx_id}")

    def _call(self, method: str, path: str, payload: dict | None = None) -> dict:
        body = None
        headers = {}
        if payload is not None:
            body = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json"

        http_request = request.Request(f"{self.base_url}{path}", data=body, headers=headers, method=method)
        try:
            with request.urlopen(http_request, timeout=5) as response:
                raw_body = response.read().decode("utf-8")
                if not raw_body:
                    return {}
                return json.loads(raw_body)
        except HTTPError as error:
            raw_body = error.read().decode("utf-8")
            payload = json.loads(raw_body) if raw_body else {}
            raise AUSFError(error.code, payload) from error