---
name: Release candidate
about: Track a release candidate, validation status, and go/no-go decision
title: "[RC]: "
labels: [release]
assignees: []
---

## Release Summary

- Candidate version:
- Target date:
- Release owner:

## Included Scope

- Related PRs:
- Changed modules:
- Changed flows:
- Backward compatibility impact: yes / no

## Validation Status

- Targeted module validation: pending / passed / failed
- Targeted smoke validation: pending / passed / failed
- Full validation (`python automation/scripts/run_full_validation.py`): pending / passed / failed / waived
- Critical happy path: pending / passed / failed
- Critical negative path: pending / passed / failed

## Evidence

Commands used:

```text
Paste the exact validation commands.
```

Key results:

```text
Summarize the important pass/fail output.
```

## Risks

- Auth state risk:
- Restart / TTL risk:
- TLS / HTTPS risk:
- Environment risk:
- Known coverage gaps:

## Release Gate

- [ ] PR template completed for included changes.
- [ ] Release note drafted from `.github/RELEASE_NOTE_TEMPLATE.md`.
- [ ] Release gate reviewed from `.github/RELEASE_GATE_CHECKLIST.md`.
- [ ] Rollback approach is known.
- [ ] Final go/no-go decision recorded.

## Decision

- Status: GO / NO-GO / HOLD
- Approver:
- Notes:
