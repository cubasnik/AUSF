# Contributing

This repository is organized by runtime layer, and changes are expected to stay close to the owning layer:

- `networking/` for C++ packet-oriented and transport-near code
- `control-plane/` for Java authentication orchestration and UDM-facing logic
- `microservices/` for the Go AUSF SBI service
- `automation/` for smoke tests, validation scripts, and operational helpers

## Before you change code

Use the issue and planning templates in `.github/` to describe the work before widening the scope:

- `bug_report.md` for one reproducible defect
- `feature_request.md` for a new behavior or contract
- `test_scenario_request.md` for missing smoke or regression coverage
- `release_candidate.md` for release sign-off tracking

Choose one primary workflow label and add flow or risk labels only when they improve routing. The canonical label catalog lives in `.github/labels.json`.

## Development expectations

- Keep changes scoped to one clear objective.
- Fix the owning layer instead of compensating in adjacent layers when possible.
- Add or update focused validation for the changed behavior.
- Avoid mixing unrelated refactors into bugfixes or release prep.
- Document residual risks when behavior changes touch auth state, callbacks, TLS, TTL, or restart persistence.

## Validation expectations

Run the narrowest useful checks first.

Typical module-level checks:

```text
cmake -S networking -B networking/build
cmake --build networking/build

cd control-plane
mvn test

cd microservices
go test ./...
```

For end-to-end or compose-backed behavior, prefer a targeted smoke script from `automation/scripts/` before running the full suite.

Full regression entrypoint:

```text
python automation/scripts/run_full_validation.py
```

If a change needs GitHub labels synchronized, use:

```powershell
pwsh ./automation/scripts/sync_github_labels.ps1
```

This requires GitHub CLI authentication.

## Pull requests

Use `.github/PULL_REQUEST_TEMPLATE.md` for every PR. A ready PR should include:

- scope and owning layer
- behavior before and after
- exact commands run for validation
- happy path and critical negative path coverage
- residual risks and operational notes

Branch protection on `main` requires the `validate` workflow to pass before merge.

## Release preparation

For release work, use these files together:

- `.github/RELEASE_NOTE_TEMPLATE.md`
- `.github/RELEASE_GATE_CHECKLIST.md`
- `.github/ISSUE_TEMPLATE/release_candidate.md`

Do not sign off a release candidate without explicit validation evidence and a recorded go/no-go decision.
