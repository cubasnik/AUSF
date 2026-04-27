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
- Java control-plane service with mock UDM/ARPF logic and file-backed subscriber storage.
- Go AUSF microservice exposing a concrete `nausf-auth` style API and delegating challenge/confirmation to Java.
- Python automation client and smoke-test helpers for the Go service contract.
- Root orchestration via `Makefile` and container startup via `docker-compose.yml`.

## Implemented AUSF flow

Current layering is now explicit:

1. The external client talks to the Go SBI service.
2. The Go service creates an external `authCtxId` and delegates challenge/confirm operations to the Java control-plane.
3. The Java control-plane loads subscriber profiles from a file-backed store and simulates UDM/ARPF vector generation.
4. The Go service returns 3GPP-shaped DTOs, including `ProblemDetails` on errors.

The Go service exposes a simplified AUSF flow under `nausf-auth`:

1. `POST /nausf-auth/v1/ue-authentications`
2. `GET /nausf-auth/v1/ue-authentications/{authCtxId}`
3. `POST /nausf-auth/v1/ue-authentications/{authCtxId}/5g-aka-confirmation`
4. `POST /nausf-auth/v1/ue-authentications/{authCtxId}/eap-session`
5. `DELETE /nausf-auth/v1/ue-authentications/{authCtxId}`

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
    "payload": "EAP-Request/AKA'-Challenge ...",
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

Current Java -> Go -> AUSF error contract:

| Cause | Java control-plane status | AUSF API status | Where it applies |
| --- | --- | --- | --- |
| `SUBSCRIBER_NOT_FOUND` | `404` | `404` | initiate when the UDM-backed subscriber lookup returns no profile |
| `AUTHENTICATION_REJECTED` | `401` | `401` | confirm when `resStar` or `eapPayload` verification fails |
| `CONTEXT_NOT_FOUND` | `404` | `404` | confirm when the Java auth context is missing or expired, or when the Go auth context is absent |
| `CONTROL_PLANE_UNAVAILABLE` | n/a | `502` | initiate or confirm when the Go AUSF service cannot reach the Java control-plane or cannot decode a usable upstream response |

Important note:

- The crypto is intentionally simplified for development. It is not a standards-compliant Milenage or TUAK implementation.
- `RAND`, `AUTN`, `RES*`, `HXRES*`, `KSEAF`, and EAP payloads are modeled to support flow development and API integration, not production-grade security.

## Project structure

```text
AUSF/
├── automation/
│   ├── requirements.txt
│   ├── scripts/
│   │   └── smoke_test.py
│   ├── src/
│   │   └── smoke_client.py
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
python scripts/smoke_test_http_udm_invalid_notification_uri.py
python scripts/smoke_test_http_udm_missing_context.py
python scripts/smoke_test_http_udm_missing_subscriber.py
python scripts/smoke_test_http_udm_authentication_rejected.py
python scripts/smoke_test_http_udm_eap_authentication_rejected.py
python scripts/run_http_udm_smoke_suite.py
python scripts/run_full_validation.py
```

The HTTP UDM smoke coverage is split by auth mode:

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
make validate-all
make compose-up
make compose-refresh-mocks
```

If you work on Windows without `make`, use Git Bash, MSYS2, WSL, or run the equivalent commands manually.

For the bind-mounted Python mock services, a plain `docker compose up -d` does not restart an already running container, so code changes in `mock-amf` or `mock-nrf` may not be picked up immediately. Use `make compose-refresh-mocks` or run `pwsh -File automation/scripts/refresh_mock_services.ps1` to force a clean stop/remove/recreate cycle for those two services.

To run the full HTTP UDM happy/negative validation set in one shot, use `make http-udm-smoke-suite` or `python automation/scripts/run_http_udm_smoke_suite.py`. The suite brings the compose stack up, runs all HTTP UDM smoke scenarios sequentially, and always tears the stack down at the end.

To run the current CI-friendly validation set in one shot, use `make validate-all` or `python automation/scripts/run_full_validation.py`. This wrapper runs Python unit tests, focused Go tests in the pinned Go devcontainer image, focused Java tests in the pinned Java 25 devcontainer image, and then the full HTTP UDM happy/negative smoke suite.

## Docker Compose

The repository includes `docker-compose.yml` for the Go AUSF service and Java control-plane service.

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

## What is still missing

This workspace is intentionally a foundation. The following are not implemented yet:

- Real 3GPP-compliant Milenage or TUAK algorithms.
- Real Nnrf / Namf interactions.
- Production-grade Nudm interoperability beyond the current pluggable mock/http development contract.
- Durable database-backed storage instead of JSON file storage.
- Production-grade security, TLS, OAuth2, and SBI authorization.
- Full 5G AKA and EAP-AKA' state machines.
- Real PFCP data plane integration.
- Circuit breaking and tracing between Go and Java services.

## Validation status

Validated in the current environment:

- C++ networking layer builds and its sample executable runs.
- Focused Go tests pass in a containerized Go toolchain, including `./internal/api`, `./internal/controlplane`, `./internal/namf`, and `./internal/service`.
- Focused Java tests pass in a containerized Java/Maven toolchain, including `AuthenticationManagerTest`, `AuthenticationControllerTest`, `NnrfClientTest`, and `HttpUdmClientTest`.
- Python automation unit tests pass.
- IDE diagnostics for the edited Go and Java sources are clean.
- Docker Compose stack with `mock-nrf` and `mock-udm` starts successfully.
- `python automation/scripts/smoke_test_http_udm.py` passes against the HTTP UDM mode.
- `python automation/scripts/smoke_test_http_udm_eap.py` passes against the HTTP UDM EAP mode.
- `python automation/scripts/smoke_test_http_udm_invalid_notification_uri.py` validates the invalid-notification create path in the same compose environment.
- `python automation/scripts/smoke_test_http_udm_missing_context.py` validates the missing-context confirmation path in the same compose environment.
- `python automation/scripts/smoke_test_http_udm_missing_subscriber.py` validates the missing-subscriber initiate path in the same compose environment.
- `python automation/scripts/smoke_test_http_udm_authentication_rejected.py` validates the authentication-rejected 5G AKA confirmation path in the same compose environment.
- `python automation/scripts/smoke_test_http_udm_eap_authentication_rejected.py` validates the authentication-rejected EAP confirmation path in the same compose environment.
- `python automation/scripts/run_http_udm_smoke_suite.py` runs the full HTTP UDM happy/negative smoke suite and cleans the compose stack up afterward.
- `python automation/scripts/run_full_validation.py` runs the current CI-friendly validation stack end-to-end: Python unit tests, focused Go tests, focused Java tests, and the HTTP UDM smoke suite.
- The HTTP UDM smoke suite validates mock Namf southbound notification paths for both `5G_AKA` and `EAP_AKA_PRIME`, and confirms the negative create/confirm paths do not emit stray callbacks.

Not fully validated in the current environment:

- Full end-to-end validation of every branch and failure mode has not been run.
- Host-native `go` and `mvn` commands were not used directly; validation was performed through containerized toolchains.
