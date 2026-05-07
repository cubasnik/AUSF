## Summary

- What changed?
- Why was this change needed?
- Which layer owns the behavior? (`networking` / `control-plane` / `microservices` / `automation`)

## Change Type

- [ ] Bug fix
- [ ] New feature
- [ ] Refactoring
- [ ] Test-only change
- [ ] Build / CI / tooling
- [ ] Release preparation

## Scope

- Affected modules:
  - [ ] `networking`
  - [ ] `control-plane`
  - [ ] `microservices`
  - [ ] `automation`
  - [ ] `docker-compose`
  - [ ] `mock-network-functions`

- Affected flows:
  - [ ] 5G-AKA
  - [ ] EAP-AKA'
  - [ ] TLS / HTTPS
  - [ ] TTL expiration
  - [ ] Restart persistence
  - [ ] NRF / UDM integration
  - [ ] Other:

## Behavior

### Before

Describe the previous behavior or failure mode.

### After

Describe the expected behavior after this change.

## Validation

### Local checks run

- [ ] `cmake -S networking -B networking/build`
- [ ] `cmake --build networking/build`
- [ ] `mvn test` in `control-plane`
- [ ] `go test ./...` in `microservices`
- [ ] Targeted Python / smoke script in `automation/scripts`

Commands run:

```text
Paste the exact commands you ran.
```

Key results:

```text
Summarize the important pass/fail output.
```

### Compose / integration checks

- [ ] Targeted smoke scenario
- [ ] `python automation/scripts/run_full_validation.py`
- [ ] Not applicable

Scenarios covered:

- Happy path:
- Critical negative path:
- Regression-adjacent scenarios:

## Risks

- Auth state transition risk:
- Repeated confirmation risk:
- TLS / transport risk:
- Restart / TTL risk:
- External dependency risk:

## Operational Notes

- Required environment or toolchain changes:
- Docker Desktop Linux engine required: yes / no
- Config changes or new env vars:
- Data / persistence impact:

## Release Notes Input

User-visible summary:

```text
One short paragraph suitable for release notes.
```

## Checklist

- [ ] The change is scoped to the owning layer.
- [ ] A reproducer or acceptance scenario exists.
- [ ] The same failing slice was revalidated after the fix.
- [ ] Neighboring regressions were checked when behavior changed.
- [ ] Logs / screenshots were added when they clarify behavior.
- [ ] Documentation was updated if contract or operations changed.
- [ ] Residual risks are called out explicitly.
