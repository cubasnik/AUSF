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
4. `DELETE /nausf-auth/v1/ue-authentications/{authCtxId}`

Supported authentication modes:

- `5G_AKA`
- `EAP_AKA_PRIME`

Example create request:

```json
{
  "supiOrSuci": "imsi-001010000000001",
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "authType": "5G_AKA"
}
```

Example `5G_AKA` response:

```json
{
  "authCtxId": "auth-1",
  "supi": "imsi-001010000000001",
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
  "supi": "imsi-001010000000002",
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "authType": "EAP_AKA_PRIME",
  "eapSession": {
    "method": "EAP-AKA'",
    "payload": "EAP-Request/AKA'-Challenge ...",
    "sessionId": "auth-2"
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
├── docker-compose.yml
├── Makefile
└── README.md
```

## How the layers fit together

- `networking/` is the place for PFCP or other transport-near protocol work.
- `control-plane/` owns subscriber lookup, authentication vectors, EAP/AKA branching, and internal auth context state.
- `microservices/` is the northbound AUSF SBI surface and now acts as an adapter over the Java control-plane.
- `automation/` is the place for smoke tests, API validation, CI helpers, and deployment scripts.

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
```

## Root orchestration

The root `Makefile` provides a single entry point for common actions:

```bash
make networking-build
make control-plane-test
make microservices-build
make automation-test
make compose-up
```

If you work on Windows without `make`, use Git Bash, MSYS2, WSL, or run the equivalent commands manually.

## Docker Compose

The repository includes `docker-compose.yml` for the Go AUSF service and Java control-plane service.

Start the stack:

```bash
docker compose up --build
```

Services:

- Go AUSF service: `http://localhost:8080`
- Java control-plane service: `http://localhost:8081`

Important runtime variables:

- `CONTROL_PLANE_BASE_URL` tells Go where the Java control-plane lives.
- `AUSF_SUBSCRIBER_STORE` tells Java where the persistent subscriber JSON file lives.

## What is still missing

This workspace is intentionally a foundation. The following are not implemented yet:

- Real 3GPP-compliant Milenage or TUAK algorithms.
- Real Nudm / Nnrf / Namf interactions.
- Durable database-backed storage instead of JSON file storage.
- Production-grade security, TLS, OAuth2, and SBI authorization.
- Full 5G AKA and EAP-AKA' state machines.
- Real PFCP data plane integration.
- Retry, circuit breaking, and tracing between Go and Java services.

## Validation status

Validated in the current environment:

- C++ networking layer builds and its sample executable runs.
- Python automation unit tests pass.
- IDE diagnostics for the edited Go and Java sources are clean.

Not fully validated in the current environment:

- Go build could not be executed here because `go` is not installed on `PATH`.
- Java Maven build could not be executed here because `mvn` is not installed on `PATH`.
- Docker Compose stack was not started here.
