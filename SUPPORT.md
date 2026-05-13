# Support

Use the repository templates before opening general support requests.

## Where to start

- Reproducible defect: use `.github/ISSUE_TEMPLATE/bug_report.md`
- New capability or contract change: use `.github/ISSUE_TEMPLATE/feature_request.md`
- Missing smoke or regression coverage: use `.github/ISSUE_TEMPLATE/test_scenario_request.md`
- Release readiness or go/no-go thread: use `.github/ISSUE_TEMPLATE/release_candidate.md`

## What to include in support requests

For any request, include enough context to identify the owning layer:

- `networking`, `control-plane`, `microservices`, `automation`, or compose/runtime
- exact command or smoke script used
- expected result and actual result
- whether the problem reproduces locally, in compose, or in CI
- relevant logs, `ProblemDetails`, or container names

## Validation-first support flow

Before asking for help on runtime behavior, prefer this order:

1. Run the narrowest local check for the affected layer.
2. Run the nearest targeted smoke scenario from `automation/scripts/`.
3. Escalate to `python automation/scripts/run_full_validation.py` only when the issue is integration-scoped.

This keeps support requests concrete and avoids widening the search before the owning layer is known.
