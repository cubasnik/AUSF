---
name: Test scenario request
about: Request a new smoke, regression, TLS, restart, or negative-path validation scenario
title: "[Test Scenario]: "
labels: [testing]
assignees: []
---

## Summary

Briefly describe the scenario that should be covered.

## Target Behavior

- Layer: `networking` / `control-plane` / `microservices` / `automation` / `docker-compose`
- Flow: `5G_AKA` / `EAP_AKA_PRIME` / `TLS / HTTPS` / `TTL` / `restart persistence` / `NRF / UDM` / other
- Scenario type: happy path / negative path / repeated confirm / stale response / sync-failure / restart / TTL / authz / TLS

## Motivation

What regression, risk, or acceptance gap should this scenario cover?

## Expected Outcome

Describe the expected status code, auth state transition, callback behavior, or persistence behavior.

## Suggested Validation Shape

- Preferred level: unit / integration / compose smoke / full validation
- Suggested script or location:

```text
Example: automation/scripts/smoke_test_http_udm_<scenario>.py
```

- Related existing scenarios:

## Acceptance Checks

1. 
2. 
3. 

## Risks

- Existing scenario most likely to regress:
- Environment dependency risk:
- Need for Docker Desktop Linux engine: yes / no

## Notes

Add payload examples, expected `ProblemDetails`, related bug reports, or linked PRs if relevant.
