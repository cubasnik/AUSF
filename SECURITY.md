# Security Policy

## Reporting a vulnerability

Do not open a public issue for a suspected security vulnerability until the impact is understood.

If you find a security-sensitive problem in this repository, report it privately to the repository owner or maintainers first. Public issues are acceptable only after the exposure, impact, and mitigation plan are already understood and it is safe to discuss the issue openly.

Include at minimum:

- affected layer or component
- reproduction steps
- expected and actual behavior
- whether the issue exposes credentials, bearer tokens, TLS trust, or subscriber data
- whether the issue affects only local development or also compose and CI validation flows

## Security-sensitive areas in this repository

Treat changes in these areas as security-sensitive by default:

- TLS certificates, trust stores, CA files, and HTTPS configuration
- bearer-token authorization for SBI, control-plane, and Namf calls
- subscriber data, sequence numbers, and authentication material
- Docker compose environment variables and secret-like values
- mock network functions that simulate authentication responses

## Handling secrets and credentials

- Do not commit real bearer tokens, private keys, or production certificates.
- Treat generated local TLS assets as development-only material.
- Prefer environment variables or local untracked files for sensitive values.
- If a secret is committed accidentally, rotate it before relying on history cleanup.

## Validation expectations for security changes

For security-sensitive changes, capture explicit validation evidence in the PR:

- targeted local tests for the changed module
- targeted smoke coverage for the affected auth or TLS flow
- full validation when the change affects shared runtime behavior or compose wiring

Security changes should also document residual risk and rollback expectations in the PR and release note templates.
