# AUSF 5G Core Polyglot Workspace

This repository contains a split-by-layer AUSF workspace for a 5G core implementation. The goal is to keep each concern in the language that best fits it:

| Layer | Main purpose | Language |
| --- | --- | --- |
| Networking / PFCP / low-level | Packet-oriented and transport-near code | C++ |
| Control plane logic | Authentication orchestration, UDM-facing vector generation, subscriber state | Java |
| Cloud / microservices | SBI-facing AUSF HTTP service | Go |
| Automation | Smoke tests, integration helpers, ops scripts | Python |

## Current scope

The project is a working foundation, not a complete production AUSF. It currently includes:

- C++ PFCP/networking sample with buildable CMake target.
- Java control-plane service with mock UDM/ARPF orchestration, standards-backed Milenage/TUAK vector generation, and PostgreSQL-backed subscriber storage.
- Go AUSF microservice exposing a concrete `nausf-auth` style API and delegating challenge/confirmation to Java.
- Auth context TTL expiration: contexts are automatically invalidated after a configurable number of seconds (`AUSF_AUTH_CONTEXT_TTL_SECONDS`). Expired contexts return `404 CONTEXT_NOT_FOUND`.
- Optional file-backed auth context persistence (`AUSF_AUTH_CONTEXT_STORE_FILE`): a running AUSF can reload its in-flight contexts after a restart, allowing confirmation to succeed even after a container restart.
- Python automation client and smoke-test helpers for the Go service contract, including context persistence and TTL scenarios.
- Root orchestration via `Makefile` and container startup via `docker-compose.yml`.
- Branch protection on `main` requires the `validate` CI check to pass before any PR can be merged.

## Contribution templates

See `CONTRIBUTING.md` for the contributor workflow, validation expectations, and PR/release preparation guidance.
See `SECURITY.md` for vulnerability handling and `SUPPORT.md` for routing general support requests.

Use the repository templates in `.github/` when preparing change reviews or release summaries:

- `.github/PULL_REQUEST_TEMPLATE.md` captures scope, affected AUSF flows, validation commands, and residual risks for PRs.
- `.github/RELEASE_NOTE_TEMPLATE.md` provides a release-ready summary with validation evidence, environment notes, and go/no-go checks.
- `.github/RELEASE_GATE_CHECKLIST.md` is a QA and ops oriented release gate checklist for final validation, risk review, and sign-off.
- `.github/ISSUE_TEMPLATE/bug_report.md` is for reproducible runtime, contract, or validation defects.
- `.github/ISSUE_TEMPLATE/feature_request.md` is for proposing new behavior, API changes, or validation coverage.
- `.github/ISSUE_TEMPLATE/release_candidate.md` is for release-candidate tracking, validation status, and go/no-go decisions.
- `.github/ISSUE_TEMPLATE/test_scenario_request.md` is for requesting new smoke, regression, TLS, restart, or negative-path coverage.

These templates are intended to keep bugfix, feature, regression, and release documentation aligned with the actual validation workflow in this workspace.

### How to use the templates

- Use `.github/ISSUE_TEMPLATE/bug_report.md` when you have one failing scenario, one expected result, and one actual result that can be reproduced locally, in compose, or in CI.
- Use `.github/ISSUE_TEMPLATE/feature_request.md` when you want to define a new capability together with acceptance criteria and expected validation scope.
- Use `.github/ISSUE_TEMPLATE/test_scenario_request.md` when behavior is already understood but coverage is missing and you need a new targeted validation scenario.
- Use `.github/PULL_REQUEST_TEMPLATE.md` when the code change exists and you need to document scope, validation, and residual risks for review.
- Use `.github/RELEASE_NOTE_TEMPLATE.md` when the change set is accepted and you need a user-facing or operator-facing release summary.
- Use `.github/RELEASE_GATE_CHECKLIST.md` and `.github/ISSUE_TEMPLATE/release_candidate.md` when preparing a candidate build for sign-off.

Minimal examples:

```text
Bug report: "POST /nausf-auth/v1/ue-authentications/{authCtxId}/5g-aka-confirmation returns 401 after a valid refreshed RES* in the sync-failure flow; expected 200 SUCCESS. Reproduced with python automation/scripts/smoke_test_http_udm_sync_failure.py"

Feature request: "Add smoke coverage for a repeated TTL-expired confirmation attempt so the same authCtxId proves 404 CONTEXT_NOT_FOUND after expiry and does not emit a Namf callback."

Test scenario request: "Add a targeted compose smoke test that proves repeated EAP confirmation after terminal FAILED returns 401 AUTHENTICATION_REJECTED and preserves EAP-Failure in the stored context."

PR summary: "Fix stale auth-state overwrite in sync-failure confirmation handling for 5G_AKA; validated with mvn test, go test ./..., and targeted smoke coverage."

Release summary: "This release stabilizes repeated confirmation and sync-failure handling in AUSF authentication flows and updates validation evidence for compose-based regression."
```

### Recommended labels

Use one primary workflow label and add optional focus labels only when they improve routing or triage.

The canonical label catalog lives in `.github/labels.json` and can be synced to GitHub with:

```powershell
pwsh ./automation/scripts/sync_github_labels.ps1
```

If you need a different target repository, pass `-Repository owner/repo`.

- Workflow labels: `bug`, `enhancement`, `testing`, `release`
- Change-shape labels: `docs`, `ci`, `ops`, `security`
- Flow labels: `5g-aka`, `eap-aka-prime`, `tls`, `ttl`, `restart-persistence`, `nrf-udm`
- Risk labels: `regression-risk`, `breaking-change`, `needs-smoke`, `needs-full-validation`

Suggested usage:

- A reproducible defect in an existing path: `bug` plus one flow label and optionally `regression-risk`
- A new capability with acceptance criteria: `enhancement` plus one flow label and optionally `breaking-change`
- A missing smoke or regression path: `testing` plus `needs-smoke` or `needs-full-validation`
- A release candidate or sign-off thread: `release` plus the highest-risk flow labels involved

## Implemented AUSF flow

Current layering is now explicit:

1. The external client talks to the Go SBI service.
2. The Go service creates an external `authCtxId` and delegates challenge/confirm operations to the Java control-plane.
3. The Java control-plane loads subscriber profiles from PostgreSQL and issues Milenage/TUAK-backed authentication vectors through the current mock or HTTP UDM contract.
4. The Go service returns 3GPP-shaped DTOs, including `ProblemDetails` on errors.

The Go service exposes a simplified AUSF flow under `nausf-auth`:

1. `POST /nausf-auth/v1/ue-authentications`
2. `GET /nausf-auth/v1/ue-authentications/{authCtxId}`
3. `POST /nausf-auth/v1/ue-authentications/{authCtxId}/5g-aka-confirmation`
4. `POST /nausf-auth/v1/ue-authentications/{authCtxId}/eap-session`
5. `DELETE /nausf-auth/v1/ue-authentications/{authCtxId}`

### Protocol interaction diagram

The diagram below shows how the four runtime components interact and which protocol / interface is carried on each arrow.

```mermaid
%%{init: {
  "theme": "base",
  "themeVariables": {
    "background":            "#f5f7fa",
    "actorBkg":              "#1e3a5f",
    "actorBorder":           "#4fc3f7",
    "actorTextColor":        "#ffffff",
    "actorLineColor":        "#90caf9",
    "signalColor":           "#1565c0",
    "signalTextColor":       "#0d2444",
    "activationBkgColor":    "#bbdefb",
    "activationBorderColor": "#1565c0",
    "sequenceNumberColor":   "#ffffff",
    "noteBkgColor":          "#e3f2fd",
    "noteBorderColor":       "#42a5f5",
    "noteTextColor":         "#0d2444",
    "labelBoxBkgColor":      "#e8f5e9",
    "labelBoxBorderColor":   "#66bb6a",
    "labelTextColor":        "#1b5e20",
    "loopTextColor":         "#1b5e20"
  }
}}%%
sequenceDiagram
    autonumber
    participant AMF  as AMF<br/>(mock-amf)
    participant AUSF as AUSF Go<br/>microservice
    participant NRF  as NRF<br/>(mock-nrf)
    participant CP   as Control-Plane<br/>(Java)
    participant UDM  as UDM<br/>(mock-udm)

    Note over AUSF,NRF: Startup — Nnrf_NFManagement (HTTP PUT /nnrf-nfm/v1/nf-instances/{id})
    AUSF->>NRF: PUT /nnrf-nfm/v1/nf-instances/{id}<br/>nfType=AUSF, nfStatus=REGISTERED
    NRF-->>AUSF: 201 Created

    loop Every heartbeat-interval-seconds (default 30 s)
        AUSF->>NRF: PATCH /nnrf-nfm/v1/nf-instances/{id}<br/>nfStatus=REGISTERED
        NRF-->>AUSF: 200 OK
    end

    Note over CP,NRF: Control-plane discovers UDM — Nnrf_NFDiscovery (HTTP GET)
    CP->>NRF: GET /nnrf-disc/v1/nf-instances?target-nf-type=UDM
    NRF-->>CP: 200 OK — NF instance list with nudm-ueau apiPrefix

    Note over AMF,UDM: 5G-AKA authentication flow — Nausf_UEAuthentication (HTTP/2 SBI)
    AMF->>AUSF: POST /nausf-auth/v1/ue-authentications<br/>supiOrSuci, authType=5G_AKA
    AUSF->>CP: POST /ausf/v1/ue-authentications<br/>(internal HTTP, supi, servingNetwork)
    CP->>UDM: POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data<br/>Nudm_UEAuthentication (HTTP/2 SBI)
    UDM-->>CP: 200 OK — AuthenticationInfoResult (RAND, AUTN, XRES*, CK', IK')
    CP-->>AUSF: 200 OK — AV (RAND, AUTN, HXRES*, KAUSF)
    AUSF-->>AMF: 201 Created — UEAuthenticationCtx (authCtxId, 5gAuthData)

    AMF->>AUSF: POST /nausf-auth/v1/ue-authentications/{authCtxId}/5g-aka-confirmation<br/>resStar or auts
    AUSF->>CP: POST /ausf/v1/ue-authentications/{authCtxId}/confirm<br/>(internal HTTP)
    alt RES* accepted
      CP-->>AUSF: 200 OK — result=SUCCESS, KSEAF
      AUSF-->>AMF: 200 OK — ConfirmationData (authResult=SUCCESS, kseaf)
      AUSF-)AMF: POST {notificationUri}<br/>Namf_Communication — auth status callback
    else AUTS / re-sync requested
      CP-->>AUSF: 200 OK — refreshed challenge (RAND, AUTN, HXRES*)
      AUSF-->>AMF: 200 OK — ConfirmationData (authResult=SYNC_FAILURE, 5gAuthData)
    end

    Note over AMF,UDM: EAP-AKA' authentication flow
    AMF->>AUSF: POST /nausf-auth/v1/ue-authentications<br/>supiOrSuci, authType=EAP_AKA_PRIME
    AUSF->>CP: POST /ausf/v1/ue-authentications (internal HTTP)
    CP->>UDM: POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data
    UDM-->>CP: 200 OK — AuthenticationInfoResult
    CP-->>AUSF: 200 OK — EAP payload (EAP-Request/AKA'-Challenge with RAND, AUTN, HXRES*)
    AUSF-->>AMF: 201 Created — UEAuthenticationCtx (authCtxId, eapSession)

    AMF->>AUSF: POST /nausf-auth/v1/ue-authentications/{authCtxId}/eap-session<br/>eapPayload
    AUSF->>CP: POST /ausf/v1/ue-authentications/{authCtxId}/confirm (internal HTTP)
    alt RES* accepted
      CP-->>AUSF: 200 OK — result=SUCCESS, KSEAF
      AUSF-->>AMF: 200 OK — ConfirmationData (authResult=SUCCESS, kseaf)
      AUSF-)AMF: POST {notificationUri}<br/>Namf_Communication — auth status callback
    else re-authentication requested
      CP-->>AUSF: 200 OK — refreshed EAP challenge
      AUSF-->>AMF: 200 OK — ConfirmationData (authResult=ONGOING, eapSession)
    end

    Note over AUSF,NRF: Shutdown — Nnrf_NFManagement (HTTP DELETE)
    AUSF->>NRF: DELETE /nnrf-nfm/v1/nf-instances/{id}
    NRF-->>AUSF: 204 No Content
```

Supported authentication modes:

- `5G_AKA`
- `EAP_AKA_PRIME`

Example create request:

```json
{
  "supiOrSuci": "imsi-250010000000001",
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "authType": "5G_AKA",
  "notificationUri": "http://mock-amf:8092/namf-comm/v1/ue-authentications/{authCtxId}/status-notify"
}
```

Example `5G_AKA` response:

```json
{
  "authCtxId": "auth-1",
  "supi": "imsi-250010000000001",
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "authType": "5G_AKA",
  "5gAuthData": {
    "rand": "...",
    "autn": "...",
    "hxresStar": "..."
  },
  "status": "CHALLENGE_SENT",
  "_links": {
    "5g-aka": {
      "href": "/nausf-auth/v1/ue-authentications/auth-1/5g-aka-confirmation"
    }
  },
  "createdAt": "2026-04-25T12:00:00Z"
}
```

Example `EAP_AKA_PRIME` response:

```json
{
  "authCtxId": "auth-2",
  "supi": "imsi-250010000000002",
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "authType": "EAP_AKA_PRIME",
  "eapSession": {
    "method": "EAP-AKA'",
    "payload": "EAP-Request/AKA'-Challenge RAND=... AUTN=... HXRES*=...",
    "sessionId": "auth-2"
  },
  "_links": {
    "eap-session": {
      "href": "/nausf-auth/v1/ue-authentications/auth-2/eap-session"
    }
  },
  "status": "CHALLENGE_SENT"
}
```

Example `ProblemDetails` error:

```json
{
  "type": "https://example.com/problem/400",
  "title": "Invalid request",
  "status": 400,
  "detail": "supiOrSuci and servingNetworkName are required",
  "cause": "MANDATORY_IE_MISSING",
  "instance": "/nausf-auth/v1/ue-authentications"
}
```

## Operational contract

### Public endpoint contract

| Method and path | Success response | Failure responses |
| --- | --- | --- |
| `GET /healthz` | `200 OK`; body: `{"status":"ok"}` | No endpoint-specific `ProblemDetails` contract; failures are generic transport/runtime failures |
| `GET /metrics` | `200 OK`; body: Prometheus text exposition for AUSF HTTP request counters and latency summaries | No endpoint-specific `ProblemDetails` contract |
| `POST /nausf-auth/v1/ue-authentications` | `201 Created`; `Location` header set to `/nausf-auth/v1/ue-authentications/{authCtxId}`; body contains `authCtxId`, `supi`, `authType`, `status=CHALLENGE_SENT`, and either `5gAuthData` for `5G_AKA` or `eapSession` for `EAP_AKA_PRIME` | `400 MALFORMED_REQUEST`; `400 MANDATORY_IE_MISSING`; `400 INVALID_NOTIFICATION_URI`; `400 UNSUPPORTED_AUTH_TYPE`; `404 SUBSCRIBER_NOT_FOUND`; `502/503 CONTROL_PLANE_UNAVAILABLE`; `405 METHOD_NOT_ALLOWED` |
| `GET /nausf-auth/v1/ue-authentications/{authCtxId}` | `200 OK`; body contains the currently stored auth context for `authCtxId` | `404 CONTEXT_NOT_FOUND` |
| `POST /nausf-auth/v1/ue-authentications/{authCtxId}/5g-aka-confirmation` | `200 OK`; body contains either `authResult=SUCCESS`, `kseaf`, and confirmation `message`, or `authResult=SYNC_FAILURE`, refreshed `5gAuthData`, and re-sync `message` | `400 MALFORMED_REQUEST`; `400 MANDATORY_IE_MISSING`; `400 INVALID_CONFIRMATION_PAYLOAD`; `404 CONTEXT_NOT_FOUND`; `401 AUTHENTICATION_REJECTED`; `502/503 CONTROL_PLANE_UNAVAILABLE` |
| `POST /nausf-auth/v1/ue-authentications/{authCtxId}/eap-session` | `200 OK`; body contains either `authResult=SUCCESS`, `kseaf`, and confirmation `message`, or `authResult=ONGOING`, refreshed `eapSession`, and refresh `message` for synchronization-failure, re-authentication, or fast re-authentication | `400 MALFORMED_REQUEST`; `400 INVALID_CONFIRMATION_PAYLOAD`; `404 CONTEXT_NOT_FOUND`; `401 AUTHENTICATION_REJECTED`; `502/503 CONTROL_PLANE_UNAVAILABLE` |
| `DELETE /nausf-auth/v1/ue-authentications/{authCtxId}` | `204 No Content`; response body omitted | `404 CONTEXT_NOT_FOUND` |

Additional route behavior:

- Unknown AUSF sub-resources return `404 RESOURCE_UNKNOWN`.
- `ue-authentications` rejects malformed JSON with `400 MALFORMED_REQUEST`.
- `ue-authentications` accepts only `5G_AKA` and `EAP_AKA_PRIME` when `authType` is provided and rejects any other non-empty value with `400 UNSUPPORTED_AUTH_TYPE`.
- When `AUSF_SBI_BEARER_TOKEN` is set, all `/nausf-auth/v1/ue-authentications...` routes require `Authorization: Bearer <token>` and reject missing or invalid tokens with `401 UNAUTHORIZED`; `/healthz` and `/metrics` remain unauthenticated for operability.
- `5g-aka-confirmation` requires exactly one of `resStar` or `auts` and rejects `eapPayload` with `400 INVALID_CONFIRMATION_PAYLOAD`.
- `5g-aka-confirmation` rejects an empty JSON payload with `400 MANDATORY_IE_MISSING` and `detail="resStar or auts is required"`.
- `eap-session` requires `eapPayload` and rejects `resStar` with `400 INVALID_CONFIRMATION_PAYLOAD`.
- In the current development contract, `eapPayload` for `EAP_AKA_PRIME` must carry `EAP-Response/AKA'-Challenge RES*=<xresStar>`; a simple echo of the request challenge is rejected.
- `eap-session` also accepts minimal `EAP-Response/AKA'-Synchronization-Failure AUTS=...`, `EAP-Response/AKA'-Reauthentication ...`, and `EAP-Response/AKA'-Fast-Reauthentication ...` triggers and answers with `authResult=ONGOING` plus a refreshed `eapSession` on the same `authCtxId`.
- The current Java -> Go propagated failure causes are `SUBSCRIBER_NOT_FOUND`, `AUTHENTICATION_REJECTED`, `CONTEXT_NOT_FOUND`, and `CONTROL_PLANE_UNAVAILABLE`.
- All Go HTTP responses include `X-Trace-Id` and `traceparent` headers. Request logs include the same trace identifier in `trace_id=...` format.
- The Go control-plane HTTP client now uses a built-in circuit breaker: repeated transport/5xx failures open the breaker and subsequent calls fail fast with `503 CONTROL_PLANE_UNAVAILABLE` until the cooldown window elapses.

### Confirmed scenarios

| Scenario | Evidence | Confirmed result |
| --- | --- | --- |
| Base AUSF happy path on fresh compose startup | `python automation/scripts/smoke_test.py` | Service readiness, create challenge, and confirm success complete without an early `502` |
| `5G_AKA` happy path with Namf callback | `python automation/scripts/smoke_test_http_udm.py` | Challenge and confirmation succeed; Namf callback is emitted |
| `5G_AKA` synchronization-failure recovery path | `python automation/scripts/smoke_test_http_udm_sync_failure.py` | `5g-aka-confirmation` first returns `SYNC_FAILURE` with refreshed `5gAuthData`, persists `CHALLENGE_SENT`, emits no early Namf callback, then completes to `SUCCESS` with one Namf callback when confirmed with the refreshed `RES*` |
| `EAP_AKA_PRIME` happy path with Namf callback | `python automation/scripts/smoke_test_http_udm_eap.py` | EAP challenge and confirmation succeed; Namf callback is emitted |
| `EAP_AKA_PRIME` synchronization-failure ongoing path | `python automation/scripts/smoke_test_http_udm_eap_sync_failure.py` | `eap-session` returns `ONGOING` with a refreshed challenge, persists `CHALLENGE_SENT`, emits no early Namf callback, then completes to `SUCCESS` |
| Repeated `EAP_AKA_PRIME` synchronization-failure ongoing path | `python automation/scripts/smoke_test_http_udm_eap_sync_failure_repeated.py` | `eap-session` accepts a second synchronization-failure response after the first refresh, returns a second `ONGOING` with a newly refreshed challenge, keeps the same `authCtxId` in `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the newest `RES*` |
| `EAP_AKA_PRIME` re-authentication ongoing path | `python automation/scripts/smoke_test_http_udm_eap_reauthentication_ongoing.py` | `eap-session` returns `ONGOING` with a refreshed challenge, persists `CHALLENGE_SENT`, emits no early Namf callback, then completes to `SUCCESS` |
| Repeated `EAP_AKA_PRIME` re-authentication ongoing path | `python automation/scripts/smoke_test_http_udm_eap_reauthentication_repeated.py` | `eap-session` accepts a second re-authentication response after the first refresh, returns a second `ONGOING` with a newly refreshed challenge, keeps the same `authCtxId` in `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the newest `RES*` |
| `EAP_AKA_PRIME` fast re-authentication ongoing path | `python automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_ongoing.py` | `eap-session` returns `ONGOING` with a refreshed challenge, persists `CHALLENGE_SENT`, emits no early Namf callback, then completes to `SUCCESS` |
| Repeated `EAP_AKA_PRIME` fast re-authentication ongoing path | `python automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_repeated.py` | `eap-session` accepts a second fast re-authentication response after the first refresh, returns a second `ONGOING` with a newly refreshed challenge, keeps the same `authCtxId` in `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the newest `RES*` |
| `EAP_AKA_PRIME` stale response after repeated fast re-authentication refresh | `python automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_repeated_stale_response.py` | `eap-session` accepts a second fast re-authentication response, but replaying the `RES*` captured after the first refresh returns `401 AUTHENTICATION_REJECTED`, persists `FAILED` with `EAP-Failure`, and emits no Namf callback |
| `EAP_AKA_PRIME` stale response after repeated synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_eap_sync_failure_repeated_stale_response.py` | `eap-session` accepts a second synchronization-failure response, but replaying the `RES*` captured after the first refresh returns `401 AUTHENTICATION_REJECTED`, persists `FAILED` with `EAP-Failure`, and emits no Namf callback |
| Invalid `notificationUri` negative path | `python automation/scripts/smoke_test_http_udm_invalid_notification_uri.py` | `400 INVALID_NOTIFICATION_URI`; no Namf callback |
| Unsupported `authType` negative path | `python automation/scripts/smoke_test_http_udm_unsupported_auth_type.py` | `400 UNSUPPORTED_AUTH_TYPE`; no Namf callback |
| Missing context negative path | `python automation/scripts/smoke_test_http_udm_missing_context.py` | `404 CONTEXT_NOT_FOUND`; no Namf callback |
| Missing subscriber negative path | `python automation/scripts/smoke_test_http_udm_missing_subscriber.py` | `404 SUBSCRIBER_NOT_FOUND`; no Namf callback |
| `5G_AKA` authentication rejection | `python automation/scripts/smoke_test_http_udm_authentication_rejected.py` | `401 AUTHENTICATION_REJECTED`; no Namf callback |
| Invalid `AUTS` on initial `5G_AKA` challenge | `python automation/scripts/smoke_test_http_udm_invalid_auts.py` | `5g-aka-confirmation` rejects an invalid initial `AUTS` with `401 AUTHENTICATION_REJECTED` and `AUTS verification failed`, persists `FAILED`, and emits no Namf callback |
| Empty `5G_AKA` confirmation payload | `python automation/scripts/smoke_test_http_udm_5g_aka_missing_confirmation_payload.py` | `5g-aka-confirmation` rejects an empty JSON payload with `400 MANDATORY_IE_MISSING`; the context remains `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the valid `RES*` |
| `5G_AKA` confirmation with `eapPayload` | `python automation/scripts/smoke_test_http_udm_5g_aka_eap_payload_rejected.py` | `5g-aka-confirmation` rejects a payload containing `eapPayload` with `400 INVALID_CONFIRMATION_PAYLOAD`; the context remains `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the valid `RES*` |
| Ambiguous `5G_AKA` confirmation payload | `python automation/scripts/smoke_test_http_udm_ambiguous_confirmation_payload.py` | `5g-aka-confirmation` rejects a payload containing both `resStar` and `auts` with `400 INVALID_CONFIRMATION_PAYLOAD`; the context remains `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the valid `RES*` |
| Repeated `5G_AKA` confirmation after `FAILED` | `python automation/scripts/smoke_test_http_udm_repeat_confirm_after_failure.py` | Initial invalid confirmation returns `401 AUTHENTICATION_REJECTED`; repeated confirmation returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `FAILED` |
| Repeated `AUTS` confirmation after initial `5G_AKA` failure | `python automation/scripts/smoke_test_http_udm_repeat_auts_after_failure.py` | Initial invalid `AUTS` returns `401 AUTHENTICATION_REJECTED` with `AUTS verification failed`; replaying the same `AUTS` on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `FAILED` |
| `5G_AKA` synchronization-failure after success | `python automation/scripts/smoke_test_http_udm_sync_failure_after_success.py` | A refreshed `SYNC_FAILURE` challenge can still complete to `SUCCESS`, but any later valid refreshed `AUTS` on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `AUTHENTICATED` |
| `5G_AKA` confirmation after synchronization-failure success | `python automation/scripts/smoke_test_http_udm_repeat_confirm_after_sync_failure_success.py` | A refreshed `SYNC_FAILURE` challenge can still complete to `SUCCESS`, but any later valid refreshed `RES*` on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `AUTHENTICATED` |
| `5G_AKA` confirmation after synchronization-failure failure | `python automation/scripts/smoke_test_http_udm_repeat_confirm_after_sync_failure_failure.py` | A refreshed `SYNC_FAILURE` challenge first rejects the stale pre-refresh `AUTS` with `401 AUTHENTICATION_REJECTED`; any later valid refreshed `RES*` on the same `authCtxId` then returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `FAILED` |
| `5G_AKA` synchronization-failure after stale-response failure | `python automation/scripts/smoke_test_http_udm_sync_failure_after_failure.py` | A refreshed `SYNC_FAILURE` challenge first rejects the stale pre-refresh `RES*` with `401 AUTHENTICATION_REJECTED`; any later valid `AUTS` on the same `authCtxId` then returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `FAILED` |
| Repeated refreshed-valid `AUTS` after `5G_AKA SYNC_FAILURE` | `python automation/scripts/smoke_test_http_udm_sync_failure_repeated_valid_auts.py` | `5g-aka-confirmation` accepts a valid refreshed `AUTS` after the first `SYNC_FAILURE`, returns a second `SYNC_FAILURE` with a newly refreshed `5gAuthData`, keeps the context `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the newest `RES*` |
| Third refreshed-valid `AUTS` after repeated `5G_AKA SYNC_FAILURE` | `python automation/scripts/smoke_test_http_udm_sync_failure_third_valid_auts.py` | `5g-aka-confirmation` accepts a third valid refreshed `AUTS` after two prior `SYNC_FAILURE` refreshes, returns a third `SYNC_FAILURE` with newly refreshed `5gAuthData`, keeps the same context `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the newest `RES*` |
| Fourth refreshed-valid `AUTS` after repeated `5G_AKA SYNC_FAILURE` | `python automation/scripts/smoke_test_http_udm_sync_failure_fourth_valid_auts.py` | `5g-aka-confirmation` accepts a fourth valid refreshed `AUTS` after three prior `SYNC_FAILURE` refreshes, returns a fourth `SYNC_FAILURE` with newly refreshed `5gAuthData`, keeps the same context `CHALLENGE_SENT`, emits no early Namf callback, and still completes to `SUCCESS` when later confirmed with the newest `RES*` |
| `5G_AKA` stale `AUTS` after fourth synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_sync_failure_fourth_stale_auts.py` | `5g-aka-confirmation` accepts a fourth valid refreshed `AUTS`, but replaying the `AUTS` captured after the third refresh returns `401 AUTHENTICATION_REJECTED` with `AUTS verification failed`, persists `FAILED`, and emits no Namf callback |
| `5G_AKA` stale `AUTS` after third synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_sync_failure_third_stale_auts.py` | `5g-aka-confirmation` accepts a third valid refreshed `AUTS`, but replaying the `AUTS` captured after the second refresh returns `401 AUTHENTICATION_REJECTED` with `AUTS verification failed`, persists `FAILED`, and emits no Namf callback |
| `5G_AKA` stale response after third synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_sync_failure_third_stale_response.py` | `5g-aka-confirmation` accepts a third valid refreshed `AUTS`, but replaying the `RES*` captured after the second refresh returns `401 AUTHENTICATION_REJECTED` with `RES* verification failed`, persists `FAILED`, and emits no Namf callback |
| `5G_AKA` stale response after synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_sync_failure_stale_response.py` | `5g-aka-confirmation` first returns `SYNC_FAILURE` with refreshed `5gAuthData`; replaying the pre-refresh `RES*` returns `401 AUTHENTICATION_REJECTED`, persists `FAILED`, and emits no Namf callback |
| `5G_AKA` stale `AUTS` after synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_sync_failure_stale_auts.py` | `5g-aka-confirmation` first returns `SYNC_FAILURE` with refreshed `5gAuthData`; replaying the pre-refresh `AUTS` returns `401 AUTHENTICATION_REJECTED` with `AUTS verification failed`, persists `FAILED`, and emits no Namf callback |
| `5G_AKA` stale `AUTS` after repeated synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_sync_failure_repeated_stale_auts.py` | `5g-aka-confirmation` accepts a second valid refreshed `AUTS`, but replaying the `AUTS` captured after the first refresh returns `401 AUTHENTICATION_REJECTED` with `AUTS verification failed`, persists `FAILED`, and emits no Namf callback |
| `5G_AKA` stale response after repeated synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_sync_failure_repeated_stale_response.py` | `5g-aka-confirmation` accepts a second valid refreshed `AUTS`, but replaying the `RES*` captured after the first refresh returns `401 AUTHENTICATION_REJECTED` with `RES* verification failed`, persists `FAILED`, and emits no Namf callback |
| `EAP_AKA_PRIME` authentication rejection | `python automation/scripts/smoke_test_http_udm_eap_authentication_rejected.py` | `401 AUTHENTICATION_REJECTED`; no Namf callback |
| Repeated `5G_AKA` confirmation after `SUCCESS` | `python automation/scripts/smoke_test_http_udm_repeat_confirm_after_success.py` | First confirmation succeeds and emits one Namf callback; repeated confirmation returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `AUTHENTICATED` |
| Repeated `AUTS` confirmation after `5G_AKA SUCCESS` | `python automation/scripts/smoke_test_http_udm_repeat_auts_after_success.py` | First confirmation succeeds and emits one Namf callback; replaying `AUTS` on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `AUTHENTICATED` |
| Repeated `EAP_AKA_PRIME` confirmation after `FAILED` | `python automation/scripts/smoke_test_http_udm_eap_repeat_confirm_after_failure.py` | Initial invalid EAP confirmation returns `401 AUTHENTICATION_REJECTED` with `EAP-Failure`; repeated confirmation returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `FAILED` |
| Repeated `EAP_AKA_PRIME` confirmation after `SUCCESS` | `python automation/scripts/smoke_test_http_udm_eap_repeat_confirm_after_success.py` | First EAP confirmation succeeds and emits one Namf callback; repeated confirmation on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `AUTHENTICATED` |
| `EAP_AKA_PRIME` synchronization-failure after `SUCCESS` | `python automation/scripts/smoke_test_http_udm_eap_sync_failure_after_success.py` | First synchronization-failure refresh returns `ONGOING`, confirmation with the refreshed `RES*` succeeds, and a later synchronization-failure on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `AUTHENTICATED` |
| `EAP_AKA_PRIME` synchronization-failure after `FAILED` | `python automation/scripts/smoke_test_http_udm_eap_sync_failure_after_failure.py` | First synchronization-failure refresh returns `ONGOING`, replaying the pre-refresh `RES*` returns `401 AUTHENTICATION_REJECTED` with `EAP-Failure`, and any later synchronization-failure on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `FAILED` |
| `EAP_AKA_PRIME` re-authentication after `SUCCESS` | `python automation/scripts/smoke_test_http_udm_eap_reauthentication_after_success.py` | First re-authentication refresh returns `ONGOING`, confirmation with the refreshed `RES*` succeeds, and a later re-authentication on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `AUTHENTICATED` |
| `EAP_AKA_PRIME` re-authentication after `FAILED` | `python automation/scripts/smoke_test_http_udm_eap_reauthentication_after_failure.py` | First re-authentication refresh returns `ONGOING`, replaying the pre-refresh `RES*` returns `401 AUTHENTICATION_REJECTED` with `EAP-Failure`, and any later re-authentication on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `FAILED` |
| `EAP_AKA_PRIME` fast re-authentication after `SUCCESS` | `python automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_after_success.py` | First fast re-authentication refresh returns `ONGOING`, confirmation with the refreshed `RES*` succeeds, and a later fast re-authentication on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `AUTHENTICATED` |
| `EAP_AKA_PRIME` fast re-authentication after `FAILED` | `python automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_after_failure.py` | First fast re-authentication refresh returns `ONGOING`, replaying the pre-refresh `RES*` returns `401 AUTHENTICATION_REJECTED` with `EAP-Failure`, and any later fast re-authentication on the same `authCtxId` returns `401 AUTHENTICATION_REJECTED` with `authentication context is no longer pending` while the context remains `FAILED` |
| `EAP_AKA_PRIME` stale response after repeated re-authentication refresh | `python automation/scripts/smoke_test_http_udm_eap_reauthentication_repeated_stale_response.py` | `eap-session` accepts a second re-authentication response, but replaying the `RES*` captured after the first refresh returns `401 AUTHENTICATION_REJECTED`, persists `FAILED` with `EAP-Failure`, and emits no Namf callback |
| `EAP_AKA_PRIME` stale response after fast re-authentication refresh | `python automation/scripts/smoke_test_http_udm_eap_fast_reauthentication_stale_response.py` | `eap-session` first returns `ONGOING` with a refreshed challenge; replaying the pre-refresh `RES*` returns `401 AUTHENTICATION_REJECTED`, persists `FAILED` with `EAP-Failure`, and emits no Namf callback |
| `EAP_AKA_PRIME` stale response after re-authentication refresh | `python automation/scripts/smoke_test_http_udm_eap_reauthentication_stale_response.py` | `eap-session` first returns `ONGOING` with a refreshed challenge; replaying the pre-refresh `RES*` returns `401 AUTHENTICATION_REJECTED`, persists `FAILED` with `EAP-Failure`, and emits no Namf callback |
| `EAP_AKA_PRIME` stale response after synchronization-failure refresh | `python automation/scripts/smoke_test_http_udm_eap_sync_failure_stale_response.py` | `eap-session` first returns `ONGOING` with a refreshed challenge; replaying the pre-refresh `RES*` returns `401 AUTHENTICATION_REJECTED`, persists `FAILED` with `EAP-Failure`, and emits no Namf callback |
| Auth context survives Go service restart | `python automation/scripts/smoke_test_http_udm_context_survives_restart.py` | Challenge created, Go container restarted, confirmation succeeds using the file-backed context store |
| Auth context TTL expiration | `python automation/scripts/smoke_test_http_udm_context_ttl_expired.py` | Challenge created with short TTL, wait for expiry, confirmation returns `404 CONTEXT_NOT_FOUND` |
| Upstream UDM unavailable | `python automation/scripts/smoke_test_http_udm_upstream_unavailable.py` | Create request for `imsi-250010000000503` returns `502 CONTROL_PLANE_UNAVAILABLE` |

### Endpoint to validation matrix

| Method and path | Focused unit coverage | Compose smoke coverage |
| --- | --- | --- |
| `GET /healthz` | Python client tests cover the health client path | All smoke scripts gate on service health before continuing |
| `POST /nausf-auth/v1/ue-authentications` | Go HTTP tests cover malformed JSON, mandatory fields, invalid `notificationUri`, unsupported `authType`, subscriber-not-found propagation, and control-plane unavailability | Happy-path create is covered by `smoke_test.py`, `smoke_test_http_udm.py`, and `smoke_test_http_udm_eap.py`; negative create failures are covered by `smoke_test_http_udm_invalid_notification_uri.py`, `smoke_test_http_udm_unsupported_auth_type.py`, and `smoke_test_http_udm_missing_subscriber.py` |
| `GET /nausf-auth/v1/ue-authentications/{authCtxId}` | Go HTTP tests cover missing-context lookup and Go service tests cover in-memory lookup lifecycle | Context retrieval is exercised during the happy-path smoke flow before confirmation |
| `POST /nausf-auth/v1/ue-authentications/{authCtxId}/5g-aka-confirmation` | Go HTTP tests cover invalid payload, missing context, authentication rejection, propagated control-plane not-found handling, and rejection of ambiguous `resStar` plus `auts` payloads; Go service tests cover rejection of non-pending contexts, repeated confirmation after `FAILED`, and stale `RES*` and stale `AUTS` after re-sync refresh | Happy-path confirmation is covered by `smoke_test.py` and `smoke_test_http_udm.py`; re-sync refresh and successful completion on the refreshed `RES*` are covered by `smoke_test_http_udm_sync_failure.py`; repeated refreshed-valid `AUTS` is covered by `smoke_test_http_udm_sync_failure_repeated_valid_auts.py`, `smoke_test_http_udm_sync_failure_third_valid_auts.py`, and `smoke_test_http_udm_sync_failure_fourth_valid_auts.py`; negative confirmation is covered by `smoke_test_http_udm_missing_context.py`, `smoke_test_http_udm_authentication_rejected.py`, `smoke_test_http_udm_invalid_auts.py`, `smoke_test_http_udm_5g_aka_missing_confirmation_payload.py`, `smoke_test_http_udm_5g_aka_eap_payload_rejected.py`, `smoke_test_http_udm_ambiguous_confirmation_payload.py`, `smoke_test_http_udm_repeat_confirm_after_success.py`, `smoke_test_http_udm_repeat_auts_after_success.py`, `smoke_test_http_udm_repeat_confirm_after_failure.py`, `smoke_test_http_udm_repeat_auts_after_failure.py`, `smoke_test_http_udm_sync_failure_after_success.py`, `smoke_test_http_udm_repeat_confirm_after_sync_failure_success.py`, `smoke_test_http_udm_repeat_confirm_after_sync_failure_failure.py`, `smoke_test_http_udm_sync_failure_after_failure.py`, `smoke_test_http_udm_sync_failure_stale_response.py`, `smoke_test_http_udm_sync_failure_stale_auts.py`, `smoke_test_http_udm_sync_failure_repeated_stale_auts.py`, `smoke_test_http_udm_sync_failure_repeated_stale_response.py`, `smoke_test_http_udm_sync_failure_third_stale_auts.py`, `smoke_test_http_udm_sync_failure_fourth_stale_auts.py`, and `smoke_test_http_udm_sync_failure_third_stale_response.py` |
| `POST /nausf-auth/v1/ue-authentications/{authCtxId}/eap-session` | Go HTTP tests cover invalid payload routing plus refreshed-challenge `ONGOING` responses for synchronization-failure, re-authentication, and fast re-authentication; Go service tests cover explicit `EAP_AKA_PRIME` context initialization, the same ongoing branches, stale refreshed-challenge rejection, and rejection of non-pending contexts | Happy-path EAP confirmation is covered by `smoke_test_http_udm_eap.py`; `ONGOING` synchronization-failure, repeated synchronization-failure, re-auth, repeated re-auth, fast re-auth, and repeated fast re-auth branches are covered by `smoke_test_http_udm_eap_sync_failure.py`, `smoke_test_http_udm_eap_sync_failure_repeated.py`, `smoke_test_http_udm_eap_reauthentication_ongoing.py`, `smoke_test_http_udm_eap_reauthentication_repeated.py`, `smoke_test_http_udm_eap_fast_reauthentication_ongoing.py`, and `smoke_test_http_udm_eap_fast_reauthentication_repeated.py`; negative EAP confirmation is covered by `smoke_test_http_udm_eap_authentication_rejected.py`, `smoke_test_http_udm_eap_repeat_confirm_after_failure.py`, `smoke_test_http_udm_eap_repeat_confirm_after_success.py`, `smoke_test_http_udm_eap_sync_failure_after_success.py`, `smoke_test_http_udm_eap_sync_failure_after_failure.py`, `smoke_test_http_udm_eap_reauthentication_after_success.py`, `smoke_test_http_udm_eap_reauthentication_after_failure.py`, `smoke_test_http_udm_eap_fast_reauthentication_after_success.py`, `smoke_test_http_udm_eap_fast_reauthentication_after_failure.py`, `smoke_test_http_udm_eap_reauthentication_stale_response.py`, `smoke_test_http_udm_eap_reauthentication_repeated_stale_response.py`, `smoke_test_http_udm_eap_fast_reauthentication_stale_response.py`, `smoke_test_http_udm_eap_fast_reauthentication_repeated_stale_response.py`, `smoke_test_http_udm_eap_sync_failure_stale_response.py`, and `smoke_test_http_udm_eap_sync_failure_repeated_stale_response.py` |
| `DELETE /nausf-auth/v1/ue-authentications/{authCtxId}` | Go HTTP tests cover missing-context delete and Go service tests cover delete-after-delete lifecycle behavior | Negative delete-after-missing-context is covered indirectly by the missing-context smoke flow cleanup path |

### Supporting validation

- `python -m unittest discover -s automation/tests` confirms Python client and helper behavior.
- `go test ./internal/api ./internal/controlplane ./internal/namf ./internal/service` confirms the focused Go service, API, and control-plane adapter slices.
- `mvn -q -Dtest=AuthenticationManagerTest,AuthenticationControllerTest,NnrfClientTest,HttpUdmClientTest test` confirms the focused Java control-plane slices.
- `pwsh -File automation/scripts/run_pre_push_regression.ps1` is the current one-command reproducible pre-push run.
- `.github/workflows/regression-suite.yml` runs the same validation flow on `push` and `pull_request` in GitHub Actions.

Important note:

- The Java control-plane now uses standards-backed Milenage and TUAK implementations for `RAND`, `AUTN`, `RES*`, `HXRES*`, `KAUSF`, and Milenage `AUTS` handling.
- The broader AUSF behavior remains development-scoped: EAP payload formatting, subscriber seed data, and the overall AKA / EAP-AKA' state machines are not yet full production-grade interop implementations.

## Project structure

```text
AUSF/
├── automation/
│   ├── requirements.txt
│   ├── scripts/
│   │   ├── smoke_test.py
│   │   ├── smoke_test_http_udm.py
│   │   ├── smoke_test_http_udm_eap.py
│   │   ├── smoke_test_http_udm_eap_sync_failure.py
│   │   ├── smoke_test_http_udm_eap_sync_failure_repeated.py
│   │   ├── smoke_test_http_udm_eap_sync_failure_repeated_stale_response.py
│   │   ├── smoke_test_http_udm_eap_reauthentication_ongoing.py
│   │   ├── smoke_test_http_udm_eap_reauthentication_repeated.py
│   │   ├── smoke_test_http_udm_eap_fast_reauthentication_ongoing.py
│   │   ├── smoke_test_http_udm_eap_fast_reauthentication_repeated.py
│   │   ├── smoke_test_http_udm_eap_fast_reauthentication_repeated_stale_response.py
│   │   ├── smoke_test_http_udm_invalid_notification_uri.py
│   │   ├── smoke_test_http_udm_missing_context.py
│   │   ├── smoke_test_http_udm_missing_subscriber.py
│   │   ├── smoke_test_http_udm_authentication_rejected.py
│   │   ├── smoke_test_http_udm_ambiguous_confirmation_payload.py
│   │   ├── smoke_test_http_udm_repeat_confirm_after_failure.py
│   │   ├── smoke_test_http_udm_repeat_auts_after_failure.py
│   │   ├── smoke_test_http_udm_sync_failure_after_success.py
│   │   ├── smoke_test_http_udm_repeat_confirm_after_sync_failure_success.py
│   │   ├── smoke_test_http_udm_repeat_confirm_after_sync_failure_failure.py
│   │   ├── smoke_test_http_udm_sync_failure_after_failure.py
│   │   ├── smoke_test_http_udm_sync_failure_repeated_valid_auts.py
│   │   ├── smoke_test_http_udm_sync_failure_third_valid_auts.py
│   │   ├── smoke_test_http_udm_sync_failure_fourth_valid_auts.py
│   │   ├── smoke_test_http_udm_sync_failure_fourth_stale_auts.py
│   │   ├── smoke_test_http_udm_sync_failure_third_stale_auts.py
│   │   ├── smoke_test_http_udm_sync_failure_third_stale_response.py
│   │   ├── smoke_test_http_udm_sync_failure_stale_response.py
│   │   ├── smoke_test_http_udm_sync_failure_stale_auts.py
│   │   ├── smoke_test_http_udm_sync_failure_repeated_stale_auts.py
│   │   ├── smoke_test_http_udm_sync_failure_repeated_stale_response.py
│   │   ├── smoke_test_http_udm_eap_authentication_rejected.py
│   │   ├── smoke_test_http_udm_repeat_confirm_after_success.py
│   │   ├── smoke_test_http_udm_repeat_auts_after_success.py
│   │   ├── smoke_test_http_udm_eap_repeat_confirm_after_failure.py
│   │   ├── smoke_test_http_udm_eap_repeat_confirm_after_success.py
│   │   ├── smoke_test_http_udm_eap_sync_failure_after_success.py
│   │   ├── smoke_test_http_udm_eap_sync_failure_after_failure.py
│   │   ├── smoke_test_http_udm_eap_reauthentication_after_success.py
│   │   ├── smoke_test_http_udm_eap_reauthentication_after_failure.py
│   │   ├── smoke_test_http_udm_eap_fast_reauthentication_after_success.py
│   │   ├── smoke_test_http_udm_eap_fast_reauthentication_after_failure.py
│   │   ├── smoke_test_http_udm_eap_fast_reauthentication_stale_response.py
│   │   ├── smoke_test_http_udm_eap_reauthentication_repeated_stale_response.py
│   │   ├── smoke_test_http_udm_eap_reauthentication_stale_response.py
│   │   ├── smoke_test_http_udm_eap_sync_failure_stale_response.py
│   │   ├── smoke_test_http_udm_context_survives_restart.py
│   │   ├── smoke_test_http_udm_context_ttl_expired.py
│   │   ├── smoke_test_http_udm_upstream_unavailable.py
│   │   ├── run_http_udm_smoke_suite.py
│   │   ├── run_full_validation.py
│   │   ├── run_ttl_smoke_only.py
│   │   ├── run_fast_validation.ps1
│   │   ├── run_pre_push_regression.ps1
│   │   └── refresh_mock_services.ps1
│   ├── src/
│   │   ├── compose_runtime.py
│   │   ├── smoke_client.py
│   │   └── smoke_runtime.py
│   └── tests/
│       └── test_smoke_client.py
├── control-plane/
│   ├── data/
│   │   └── subscribers.json
│   ├── Dockerfile
│   ├── pom.xml
│   └── src/
│       ├── main/
│       │   ├── java/com/ausf/controlplane/
│       │   │   ├── ControlPlaneApplication.java
│       │   │   ├── api/
│       │   │   │   └── AuthenticationController.java
│       │   │   ├── authentication/
│       │   │   │   ├── AuthenticationContext.java
│       │   │   │   ├── AuthenticationManager.java
│       │   │   │   ├── AuthenticationRequest.java
│       │   │   │   ├── AuthenticationResponse.java
│       │   │   │   └── CryptographyService.java
│       │   │   ├── subscriber/
│       │   │   │   ├── FileSubscriberRepository.java
│       │   │   │   ├── SubscriberProfile.java
│       │   │   │   └── SubscriberRepository.java
│       │   │   └── udm/
│       │   │       ├── AuthenticationVector.java
│       │   │       └── UdmService.java
│       │   └── resources/
│       │       ├── application.properties
│       │       └── seed/
│       │           └── subscribers.json
│       └── test/
│           └── java/com/ausf/controlplane/authentication/
│               └── AuthenticationManagerTest.java
├── microservices/
│   ├── Dockerfile
│   ├── cmd/
│   │   └── ausf/
│   │       └── main.go
│   ├── go.mod
│   └── internal/
│       ├── api/
│       │   ├── http_handler.go
│       │   └── problem_details.go
│       ├── config/
│       │   └── config.go
│       ├── controlplane/
│       │   └── client.go
│       └── service/
│           ├── auth_context_store.go
│           └── auth_service.go
├── networking/
│   ├── CMakeLists.txt
│   ├── Makefile
│   ├── include/
│   │   └── pfcp_handler.h
│   └── src/
│       ├── main.cpp
│       └── pfcp_handler.cpp
├── mock-network-functions/
│   ├── nrf_server.py
│   └── udm_server.py
├── docker-compose.yml
├── Makefile
└── README.md
```

## How the layers fit together

- `networking/` is the place for PFCP or other transport-near protocol work.
- `control-plane/` owns subscriber lookup, authentication vectors, EAP/AKA branching, and internal auth context state.
- `microservices/` is the northbound AUSF SBI surface and now acts as an adapter over the Java control-plane.
- `automation/` is the place for smoke tests, API validation, CI helpers, and deployment scripts.

The control-plane now supports a pluggable UDM southbound integration mode:

- `mock` mode uses the local file-backed subscriber store and development vector generation.
- `http` mode calls an external UDM-style endpoint and expects pre-generated authentication data.

For southbound integration testing, the repository also provides lightweight mock network functions:

- `mock-udm` exposes a development `Nudm_UEAuthentication`-style HTTP endpoint.
- `mock-nrf` exposes a development `Nnrf_NFDiscovery`-style HTTP endpoint that returns the `mock-udm` location.

## Local development

### C++ networking layer

```bash
cmake -S networking -B networking/build
cmake --build networking/build
./networking/build/Debug/networking_test.exe
```

### Java control plane

```bash
cd control-plane
mvn test
mvn spring-boot:run
```

Internal Java endpoints:

- `POST /control-plane/v1/auth/initiate`
- `POST /control-plane/v1/auth/{supi}/confirm`
- `GET /control-plane/v1/auth/{supi}`

### Go microservice

```bash
cd microservices
set CONTROL_PLANE_BASE_URL=http://127.0.0.1:8081
go build ./...
go run ./cmd/ausf
```

### Python automation

```bash
cd automation
python -m unittest discover -s tests
python scripts/smoke_test.py
python scripts/smoke_test_http_udm.py
python scripts/smoke_test_http_udm_eap.py
python scripts/smoke_test_http_udm_eap_sync_failure.py
python scripts/smoke_test_http_udm_eap_sync_failure_repeated.py
python scripts/smoke_test_http_udm_eap_sync_failure_repeated_stale_response.py
python scripts/smoke_test_http_udm_eap_reauthentication_ongoing.py
python scripts/smoke_test_http_udm_eap_reauthentication_repeated.py
python scripts/smoke_test_http_udm_eap_fast_reauthentication_ongoing.py
python scripts/smoke_test_http_udm_eap_fast_reauthentication_repeated.py
python scripts/smoke_test_http_udm_eap_fast_reauthentication_repeated_stale_response.py
python scripts/smoke_test_http_udm_invalid_notification_uri.py
python scripts/smoke_test_http_udm_missing_context.py
python scripts/smoke_test_http_udm_missing_subscriber.py
python scripts/smoke_test_http_udm_authentication_rejected.py
python scripts/smoke_test_http_udm_ambiguous_confirmation_payload.py
python scripts/smoke_test_http_udm_repeat_confirm_after_failure.py
python scripts/smoke_test_http_udm_repeat_auts_after_failure.py
python scripts/smoke_test_http_udm_sync_failure_after_success.py
python scripts/smoke_test_http_udm_repeat_confirm_after_sync_failure_success.py
python scripts/smoke_test_http_udm_repeat_confirm_after_sync_failure_failure.py
python scripts/smoke_test_http_udm_sync_failure_after_failure.py
python scripts/smoke_test_http_udm_sync_failure_repeated_valid_auts.py
python scripts/smoke_test_http_udm_sync_failure_third_valid_auts.py
python scripts/smoke_test_http_udm_sync_failure_fourth_valid_auts.py
python scripts/smoke_test_http_udm_sync_failure_fourth_stale_auts.py
python scripts/smoke_test_http_udm_sync_failure_third_stale_auts.py
python scripts/smoke_test_http_udm_sync_failure_third_stale_response.py
python scripts/smoke_test_http_udm_sync_failure_stale_response.py
python scripts/smoke_test_http_udm_sync_failure_stale_auts.py
python scripts/smoke_test_http_udm_sync_failure_repeated_stale_auts.py
python scripts/smoke_test_http_udm_sync_failure_repeated_stale_response.py
python scripts/smoke_test_http_udm_eap_authentication_rejected.py
python scripts/smoke_test_http_udm_repeat_confirm_after_success.py
python scripts/smoke_test_http_udm_repeat_auts_after_success.py
python scripts/smoke_test_http_udm_eap_repeat_confirm_after_failure.py
python scripts/smoke_test_http_udm_eap_repeat_confirm_after_success.py
python scripts/smoke_test_http_udm_eap_sync_failure_after_success.py
python scripts/smoke_test_http_udm_eap_sync_failure_after_failure.py
python scripts/smoke_test_http_udm_eap_reauthentication_after_success.py
python scripts/smoke_test_http_udm_eap_reauthentication_after_failure.py
python scripts/smoke_test_http_udm_eap_fast_reauthentication_after_success.py
python scripts/smoke_test_http_udm_eap_fast_reauthentication_after_failure.py
python scripts/smoke_test_http_udm_eap_fast_reauthentication_repeated_stale_response.py
python scripts/smoke_test_http_udm_eap_reauthentication_repeated_stale_response.py
python scripts/smoke_test_http_udm_eap_fast_reauthentication_stale_response.py
python scripts/smoke_test_http_udm_eap_reauthentication_stale_response.py
python scripts/smoke_test_http_udm_eap_sync_failure_stale_response.py
python scripts/smoke_test_http_udm_context_survives_restart.py
python scripts/smoke_test_http_udm_context_ttl_expired.py
python scripts/smoke_test_http_udm_upstream_unavailable.py
python scripts/run_http_udm_smoke_suite.py
python scripts/run_https_tls_smoke_suite.py
python scripts/run_full_validation.py
pwsh -File scripts/run_fast_validation.ps1
pwsh -File scripts/run_pre_push_regression.ps1
```

From the repository root, the shortest one-command validation entrypoint is:

```bash
make validate-fast
```

`make validate-fast` is the canonical make entrypoint for local validation. On Windows, that target delegates to the PowerShell wrapper `automation/scripts/run_fast_validation.ps1`, so the same fast runner is used by both `make validate-fast` and direct PowerShell execution. The shared runner then executes the optimized full validation workflow in `automation/scripts/run_full_validation.py`. When host Maven is available, it reuses the fast path that packages the Java runtime JAR on the host, prebuilds the `ausf-control-plane` and `ausf-go` runtime images, and then runs both the HTTP UDM smoke suite and the HTTPS TLS smoke suite with `--skip-build`.

On Windows hosts where `make` is not available in `PATH`, use:

```powershell
pwsh -File automation/scripts/run_fast_validation.ps1
```

To keep using the root Makefile-style entrypoints without remembering which GNU Make binary is installed, use the repository wrapper:

```powershell
.\make.ps1 validate-fast
```

The wrapper resolves `make`, `gmake`, or `mingw32-make` from `PATH` and forwards the requested target unchanged.

If MSYS2 is installed, the same Makefile target can also be run through its GNU Make-compatible executable. On this Windows host, the HTTPS TLS target was validated with:

```powershell
mingw32-make https-tls-smoke-suite
```

If you only need to rerun the compose-backed smoke suite after those images are already prepared, use:

```bash
python automation/scripts/run_http_udm_smoke_suite.py --skip-build
```

If you want to validate the opt-in TLS wiring between the Go AUSF service and the Java control-plane, use:

```bash
python automation/scripts/run_https_tls_smoke_suite.py
```

That TLS suite generates a local self-signed dev CA and service certificates under `automation/.tls-dev/`, brings the stack up with `docker-compose.tls.yml`, runs HTTPS happy-path smoke coverage for both `5G_AKA` and `EAP_AKA_PRIME` against `https://ausf-go:8080`, verifies the Java control-plane health endpoint over `https://ausf-control-plane:8081`, and then tears the stack down.

It also runs a negative TLS phase with `docker-compose.tls.invalid-control-plane-ca.yml`, intentionally gives the Go AUSF service the wrong CA for the Java control-plane, and asserts that authentication initiation fails with `CONTROL_PLANE_UNAVAILABLE` rather than silently falling back.

If you only need the TTL-expiration verification (without the rest of the smoke matrix), use:

```bash
make smoke-ttl-only
```

`make smoke-ttl-only` rebuilds `ausf-go`, starts compose without extra rebuilds, runs `automation/scripts/smoke_test_http_udm_context_ttl_expired.py`, and then tears the stack down.

`make validate-all` remains available as a backward-compatible alias for the same full workflow.

The HTTP UDM smoke coverage is split by auth mode:

- `python automation/scripts/smoke_test.py` validates the base AUSF happy path against a freshly started compose stack and now waits for service readiness before initiating authentication.
- `python automation/scripts/smoke_test_http_udm.py` validates the `5G_AKA` flow and Namf callback path.
- `python automation/scripts/smoke_test_http_udm_eap.py` validates the `EAP_AKA_PRIME` flow through the dedicated `eap-session` confirmation subresource and the same Namf callback path.
- `python automation/scripts/smoke_test_http_udm_invalid_notification_uri.py` validates the negative create path, asserting `400` with `cause=INVALID_NOTIFICATION_URI` and no Namf callback for a relative `notificationUri`.
- `python automation/scripts/smoke_test_http_udm_missing_context.py` validates the negative confirm path, asserting `404` with `cause=CONTEXT_NOT_FOUND` and no Namf callback for a missing `authCtxId`.
- `python automation/scripts/smoke_test_http_udm_missing_subscriber.py` validates the negative initiate path, asserting `404` with `cause=SUBSCRIBER_NOT_FOUND` and no Namf callback for an unknown SUPI.
- `python automation/scripts/smoke_test_http_udm_authentication_rejected.py` validates the negative confirm path, asserting `401` with `cause=AUTHENTICATION_REJECTED` and no Namf callback for an invalid `resStar`.
- `python automation/scripts/smoke_test_http_udm_eap_authentication_rejected.py` validates the negative EAP confirm path, asserting `401` with `cause=AUTHENTICATION_REJECTED` and no Namf callback for an invalid `eapPayload`.

## Root orchestration

The root `Makefile` provides a single entry point for common actions:

```bash
make networking-build
make control-plane-test
make microservices-build
make automation-test
make http-udm-smoke-suite
make https-tls-smoke-suite
make validate-all
make regression-suite
make compose-up
make compose-refresh-mocks
```

If you work on Windows without `make`, use Git Bash, MSYS2, WSL, or run the equivalent commands manually. When MSYS2 is available, `mingw32-make` is the expected drop-in replacement for these root targets.

For the bind-mounted Python mock services, a plain `docker compose up -d` does not restart an already running container, so code changes in `mock-amf` or `mock-nrf` may not be picked up immediately. Use `make compose-refresh-mocks` or run `pwsh -File automation/scripts/refresh_mock_services.ps1` to force a clean stop/remove/recreate cycle for those two services.

To run the full HTTP UDM happy/negative validation set in one shot, use `make http-udm-smoke-suite` or `python automation/scripts/run_http_udm_smoke_suite.py`. The suite brings the compose stack up, runs three happy-path smoke scenarios (`smoke_test.py`, `smoke_test_http_udm.py`, `smoke_test_http_udm_eap.py`), then the negative HTTP UDM scenarios, and always tears the stack down at the end.

To run the current minimal reproducible pre-push regression suite in one shot, use `make regression-suite`, `make validate-all`, `python automation/scripts/run_full_validation.py`, or `pwsh -File automation/scripts/run_pre_push_regression.ps1`. This wrapper runs Python unit tests, focused Go tests in the pinned Go devcontainer image, focused Java tests in the pinned Java 25 devcontainer image, then the full HTTP UDM happy/negative smoke suite, and finally the HTTPS TLS happy/negative smoke suite.

## Docker Compose

The repository includes `docker-compose.yml` for the Go AUSF service and Java control-plane service.

For local TLS validation, the repository also includes `docker-compose.tls.yml`, which overlays the base compose stack with a self-signed dev certificate bundle and enables HTTPS on the Go AUSF service and Java control-plane.

Start the stack:

```bash
docker compose up --build
```

Services:

- Go AUSF service: `http://localhost:8080`
- Java control-plane service: `http://localhost:8081`
- Mock UDM service: `http://localhost:8090`
- Mock NRF service: `http://localhost:8091`
- Mock AMF service: `http://localhost:8092`

Important runtime variables:

- `CONTROL_PLANE_BASE_URL` tells Go where the Java control-plane lives.
- `AUSF_SUBSCRIBER_STORE` tells Java where the persistent subscriber JSON file lives.
- `AUSF_UDM_MODE` selects the UDM integration mode: `mock` or `http`.
- `AUSF_UDM_BASE_URL` points the control-plane directly at an external UDM when `AUSF_UDM_MODE=http`.
- `AUSF_NNRF_BASE_URL` points the control-plane at an external NRF discovery service when the UDM location should be resolved dynamically.
- `AUSF_NAMF_BASE_URL` points the Go AUSF service at an AMF-facing status notification endpoint.
- `AUSF_SBI_BEARER_TOKEN` (optional) enables inbound bearer-token authorization on the Go AUSF SBI routes under `/nausf-auth/v1/ue-authentications`.
- `CONTROL_PLANE_BEARER_TOKEN` (optional) adds `Authorization: Bearer ...` on Go outbound calls to the Java control-plane.
- `AUSF_NAMF_BEARER_TOKEN` (optional) adds `Authorization: Bearer ...` on Go outbound Namf status notifications.
- `AUSF_TLS_CERT_FILE` and `AUSF_TLS_KEY_FILE` (optional) enable HTTPS on the Go AUSF service when both are set.
- `CONTROL_PLANE_TLS_CA_CERT_FILE` (optional) adds a PEM CA bundle for the Go service when it connects to the Java control-plane over HTTPS.
- `AUSF_NAMF_TLS_CA_CERT_FILE` (optional) adds a PEM CA bundle for Namf callback delivery over HTTPS.
- `AUSF_AUTH_CONTEXT_STORE_FILE` (optional) sets the path to a JSON file where the Go AUSF service persists in-flight auth contexts. When set, contexts survive a container restart. When unset, contexts are stored in-memory only.
- `AUSF_AUTH_CONTEXT_TTL_SECONDS` (optional, default unlimited) sets the TTL in seconds for auth contexts. Contexts older than this value are treated as expired and return `404 CONTEXT_NOT_FOUND`.
- `AUSF_CONTROL_PLANE_BREAKER_FAILURES` (optional, default `5`) sets how many consecutive transport/5xx failures are required to open the Go control-plane circuit breaker.
- `AUSF_CONTROL_PLANE_BREAKER_TIMEOUT_SECONDS` (optional, default `10`) sets how long the breaker stays open before it allows calls again.
- `AUSF_SERVER_TLS_ENABLED`, `AUSF_SERVER_TLS_KEY_STORE`, `AUSF_SERVER_TLS_KEY_STORE_PASSWORD`, and `AUSF_SERVER_TLS_KEY_STORE_TYPE` configure HTTPS for the Java control-plane server.
- `AUSF_TLS_CLIENT_CA_CERT_FILE` (optional) adds a PEM CA bundle for Java outbound HTTPS calls to NRF and UDM.

Observability endpoints and headers:

- `GET /metrics` returns Prometheus-compatible metrics for request counts and latency.
- `X-Trace-Id` is returned on every Go API response.
- `traceparent` is returned on every Go API response and accepted on incoming requests.

When `notificationUri` is provided on the create request, the Go AUSF service uses that per-session callback template in preference to the global `AUSF_NAMF_BASE_URL`. The `{authCtxId}` placeholder is replaced with the generated authentication context identifier before the Namf callback is sent.

When `AUSF_NNRF_BASE_URL` is set and `AUSF_UDM_BASE_URL` is empty, the control-plane first calls:

- `GET /nnrf-disc/v1/nf-instances?target-nf-type=UDM&requester-nf-type=AUSF`

Expected NRF response body:

```json
{
  "nfInstances": [
    {
      "services": [
        {
          "serviceName": "nudm-ueau",
          "apiPrefix": "http://mock-udm:8090"
        }
      ]
    }
  ]
}
```

When `AUSF_UDM_MODE=http`, the control-plane calls:

- `POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data`

Expected request body:

```json
{
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "authType": "5G_AKA"
}
```

When `AUSF_NAMF_BASE_URL` is set, the Go AUSF service sends a southbound notification after successful confirmation:

- `POST /namf-comm/v1/ue-authentications/{authCtxId}/status-notify`

Expected request body:

```json
{
  "authCtxId": "auth-1",
  "supi": "imsi-250010000000001",
  "authType": "5G_AKA",
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "authResult": "SUCCESS",
  "kseaf": "..."
}
```

Expected response body:

```json
{
  "supi": "imsi-250010000000001",
  "authType": "5G_AKA",
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "rand": "...",
  "autn": "...",
  "xresStar": "...",
  "hxresStar": "...",
  "kausf": "...",
  "eapChallenge": null
}
```

## Known limitations

This workspace is intentionally a development foundation. The following limitations are currently known and accepted:

- Real Nnrf / Namf interactions.
- Production-grade Nudm interoperability beyond the current pluggable mock/http development contract.
- Production-grade PostgreSQL operations such as HA, migrations, backup/restore, and secret-managed credentials.
- Mutual TLS, certificate rotation, OAuth2, and full SBI authorization hardening.
- Full 5G AKA and EAP-AKA' state machines.
- Real PFCP data plane integration.
- Circuit breaking and tracing between Go and Java services.

## Validation coverage

Operational scenarios are listed in the `Operational contract` section above. Additional coverage confirmed in the current environment:

- C++ networking layer builds and its sample executable runs.
- Focused Go tests pass in a containerized Go toolchain, including `./internal/api`, `./internal/controlplane`, `./internal/namf`, and `./internal/service`.
- Focused Java tests pass in a containerized Java/Maven toolchain, including `AuthenticationManagerTest`, `AuthenticationControllerTest`, `NnrfClientTest`, `HttpUdmClientTest`, `MilenageTest`, `TuakTest`, `UdmServiceTest`, and the subscriber persistence slice.
- Focused TLS transport tests pass for the Go AUSF client/server wiring and for the Java UDM/NRF client slice.
- `python automation/scripts/run_https_tls_smoke_suite.py` validates the compose-backed HTTPS happy paths end-to-end for both `5G_AKA` and `EAP_AKA_PRIME` and also checks the negative wrong-CA path that must fail with `CONTROL_PLANE_UNAVAILABLE`.
- `python automation/scripts/smoke_test_http_udm_authorization.py` validates opt-in bearer authorization in compose: create without token fails with `401 UNAUTHORIZED`, while the same flow succeeds with the configured bearer token.
- Python automation unit tests pass.
- IDE diagnostics for the edited Go and Java sources are clean.
- Docker Compose stack with `mock-nrf`, `mock-udm`, and `mock-amf` starts successfully.
- The compose-backed HTTP UDM suite now runs against a PostgreSQL-backed control-plane runtime and completes successfully end-to-end.
- Auth context TTL expiration smoke validated end-to-end: context created, TTL elapsed, confirmation returns `404 CONTEXT_NOT_FOUND`.
- Auth context file-backed persistence smoke validated: context survives a Go container restart, confirmation succeeds after restart.
- Upstream-unavailable smoke validated: `imsi-250010000000503` triggers `502 CONTROL_PLANE_UNAVAILABLE` via `MOCK_UDM_UNAVAILABLE_SUPIS`.
- `python automation/scripts/run_http_udm_smoke_suite.py` runs the full HTTP UDM happy/negative smoke suite and cleans the compose stack up afterward.
- `python automation/scripts/run_full_validation.py` runs the current CI-friendly validation stack end-to-end: Python unit tests, focused Go tests, focused Java tests, and the HTTP UDM smoke suite.
- `pwsh -File automation/scripts/run_pre_push_regression.ps1` runs the same suite through the Windows-oriented helper wrapper.
- `.github/workflows/regression-suite.yml` enforces the same `validate` job on every `push` and `pull_request`. The `main` branch is protected: merging requires one approved review, resolved conversations, and a passing `validate` status check.

Not fully validated in the current environment:

- Full end-to-end validation of every branch and failure mode has not been run.
- Host-native `go` and `mvn` commands were not used directly; validation was performed through containerized toolchains.

## Roadmap

The backlog below is ordered by priority. Items are grouped into three horizons.

### Horizon 1 — correctness and production-readiness (next sprint)

| # | Item | Layer | Why |
|---|------|-------|-----|
| 1 | ~~Replace development crypto with real Milenage/TUAK~~ | Java control-plane | ✅ Implemented: `UdmService` now issues Milenage/TUAK-backed vectors, Milenage `AUTS` resynchronization is wired for `5G_AKA`, and reference-vector tests cover both algorithms |
| 2 | ~~Replace embedded H2 subscriber storage with production-grade durable persistence (e.g. PostgreSQL)~~ | Java control-plane | ✅ Implemented: runtime subscriber persistence now uses PostgreSQL-backed JPA configuration plus Java seeding instead of H2-specific `data.sql` |
| 3 | ~~Add TLS between services and toward external NFs~~ | Go + Java | ✅ Implemented as opt-in HTTPS/TLS for Go server, Go outbound control-plane/Namf clients, Java control-plane server, and Java outbound UDM/NRF clients via env-configured cert/key and CA bundle settings |
| 4 | ~~Add OAuth2/token-based SBI authorization~~ | Go microservice | ✅ Implemented as opt-in bearer-token authorization on inbound Go SBI routes plus outbound bearer-token propagation from Go to the Java control-plane and Namf clients via env-configured tokens |
| 5 | Implement full 5G AKA state machine (including SYNC\_FAILURE and re-sync) | Go + Java | Minimal `AUTS -> SYNC_FAILURE -> refreshed challenge` flow is now implemented; the remaining gap is broader spec-complete state handling and failure coverage |
| 6 | Implement full EAP-AKA' state machine (EAP-Failure, sync-failure, re-auth, fast re-auth) | Go + Java | Minimal EAP failure propagation, synchronization-failure, re-authentication, and fast re-authentication refresh flows are implemented; the remaining gap is broader spec-complete state semantics rather than absence of these branches |

### Horizon 2 — resilience and observability (following sprint)

| # | Item | Layer | Why |
|---|------|-------|-----|
| 7 | Add circuit breaker between Go↔Java and Java↔UDM | Go + Java | ✅ Go↔Java implemented; ✅ Java↔UDM `UdmCircuitBreaker` implemented (configurable via `AUSF_UDM_BREAKER_FAILURES` / `AUSF_UDM_BREAKER_OPEN_SECONDS`) |
| 8 | ✅ Add distributed tracing (OpenTelemetry) | Go + Java | Correlate requests across the three-service boundary for debugging and SLA monitoring |
| 9 | ~~Expand `/metrics` coverage~~ | Go microservice | ✅ Implemented: `ausf_auth_initiated_total`, `ausf_auth_confirmed_total`, `ausf_auth_failed_total` by auth type/cause alongside HTTP metrics |
| 10 | ~~Add structured JSON logging with trace-ID propagation~~ | Go microservice | ✅ Implemented: all log lines are machine-parseable JSON `{"time","level","trace_id","msg",...}` |
| 11 | ~~Harden auth context TTL: persist TTL metadata across restarts~~ | Go microservice | ✅ Implemented: `CreatedAt` is persisted in the JSON file store; zero-timestamp legacy contexts are treated as expired when TTL is enabled |

### Horizon 3 — integration and deployment

| # | Item | Layer | Why |
|---|------|-------|-----|
| 12 | ~~Production-grade NRF integration: heartbeat registration, NF profile, subscription-based UDM discovery~~ | Java control-plane | ✅ Implemented: `NrfLifecycleManager` registers on startup via `PUT /nnrf-nfm/v1/nf-instances/{id}`, sends periodic heartbeats (`PATCH`), and deregisters on shutdown (`DELETE`). Configurable via `AUSF_NNRF_NF_INSTANCE_ID`, `AUSF_NNRF_HEARTBEAT_INTERVAL` (default 30 s) |
| 13 | Production-grade Nudm interoperability (full `Nudm_UEAuthentication` contract) | Java control-plane | Current mock UDM contract is simplified; real UDM response shapes differ |
| 14 | Real PFCP data plane integration in the C++ networking layer | C++ | Current PFCP code is a stub; connecting it to the authentication result flow closes the user-plane loop |
| 15 | ~~Kubernetes/Helm deployment manifests with readiness/liveness probes~~ | Infrastructure | ✅ Implemented: Helm chart at `deploy/helm/ausf/` — Go microservice + Java control-plane Deployments/Services, optional mock NFs (`mocks.enabled`), PVC for auth-context store, liveness/readiness probes, non-root security contexts |
| 16 | Load and soak testing with realistic SUPI populations | Automation | Verify throughput, TTL under concurrent load, and file-store write performance |
| 17 | ✅ Devcontainer-based one-click local setup | Infrastructure | Remove dependency on pre-installed Docker/Maven/Go versions on developer machines |
