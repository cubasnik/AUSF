# AUSF Release Gate Checklist

Use this checklist before approving a release candidate, tagging a build, or merging a high-risk stabilization PR.

## 1. Scope Freeze

- [ ] The release scope is explicitly listed.
- [ ] Changed modules are identified: `networking`, `control-plane`, `microservices`, `automation`, `docker-compose`, `mock-network-functions`.
- [ ] Changed auth flows are identified: `5G_AKA`, `EAP_AKA_PRIME`, `TLS / HTTPS`, `TTL`, `restart persistence`, `NRF / UDM`.
- [ ] Unrelated changes were removed or deferred.

## 2. Environment Readiness

- [ ] The toolchain versions used for validation are recorded.
- [ ] Docker and `docker compose` are available.
- [ ] Docker Desktop Linux engine is available when compose validation is required.
- [ ] Any new environment variables or config changes are documented.

## 3. Targeted Validation

- [ ] C++ changes were validated with:

```text
cmake -S networking -B networking/build
cmake --build networking/build
```

- [ ] Java changes were validated with:

```text
cd control-plane
mvn test
```

- [ ] Go changes were validated with:

```text
cd microservices
go test ./...
```

- [ ] Python automation changes were validated with a targeted script from `automation/scripts`.

## 4. Integration Validation

- [ ] The primary changed behavior has a targeted smoke scenario.
- [ ] The critical happy path was executed.
- [ ] One critical negative path was executed.
- [ ] Neighboring regression scenarios were executed for the same auth branch.
- [ ] Full validation was run when required:

```text
python automation/scripts/run_full_validation.py
```

- [ ] If full validation was skipped, the reason and compensating checks are recorded.

## 5. High-Risk Behavior Review

- [ ] Auth state transitions remain intentional.
- [ ] Repeated confirmation behavior remains intentional.
- [ ] Synchronization-failure refresh behavior remains intentional.
- [ ] Restart persistence behavior remains intentional.
- [ ] TTL expiration behavior remains intentional.
- [ ] TLS / HTTPS behavior remains intentional.
- [ ] Namf callback side effects remain intentional.

## 6. Release Evidence

- [ ] PR description is filled using `.github/PULL_REQUEST_TEMPLATE.md`.
- [ ] Release summary is filled using `.github/RELEASE_NOTE_TEMPLATE.md`.
- [ ] Commands used for validation are captured verbatim.
- [ ] Pass/fail results are summarized clearly.
- [ ] Residual risks and coverage gaps are called out explicitly.

## 7. Go / No-Go

- [ ] No unresolved blocker remains for the changed flow.
- [ ] Required validation passed or was explicitly waived.
- [ ] Rollback approach is known.
- [ ] Release owner approves go-live.
- [ ] QA / validation sign-off is recorded.
- [ ] Final decision: GO / NO-GO