## Release Note

### Title

Short release title:

### Summary

Provide a short paragraph describing what changed and why it matters.

### Scope

- Release type: `bugfix` / `feature` / `stabilization` / `release prep`
- Affected modules:
- Affected flows:
- Backward compatibility impact: yes / no

### Included Changes

| Area | Change | User-visible impact | Risk |
| --- | --- | --- | --- |
| Example: `microservices` | Fixed repeated confirmation handling after terminal auth state | Invalid repeated confirm now returns the expected error | Low |

### Validation Evidence

- Local module validation completed:
- Targeted smoke scenarios completed:
- Full compose validation completed:
- Commands used:

```text
Paste the exact commands used for release validation.
```

### Critical Scenarios Confirmed

- Happy path:
- Critical negative path:
- Restart / TTL path:
- TLS / HTTPS path:

### Environment Notes

- Java / Maven version used:
- Go version used:
- Python version used:
- Docker / Compose environment used:
- Special setup required:

### Known Risks

- Remaining functional risks:
- Infrastructure or environment risks:
- Coverage gaps:

### Rollback / Mitigation

- Rollback approach:
- Safe fallback behavior:
- Extra monitoring or smoke checks after deployment:

### Upgrade / Operator Notes

- Required config changes:
- Required data or persistence changes:
- Required deployment order:

### Final Go/No-Go

- [ ] Scope is understood and frozen.
- [ ] Targeted validations passed.
- [ ] Integration validation passed.
- [ ] Critical negative path was verified.
- [ ] Residual risks are acceptable.
- [ ] Release is approved.