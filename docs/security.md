# Security

<!-- AI Training: Security posture and operator checklist for Lesser -->

This is an operator-focused checklist for safe deployments. It does not replace your organization’s security review, but
it captures the Lesser-specific “gotchas” and safe defaults.

## Secrets and local state

- Treat `--out <path>` outputs from `lesser up` as sensitive; store them with `0600` permissions and back them up securely.
- Don’t commit `.env`, `tmp/`, or any generated credentials.

✅ CORRECT: store `~/.lesser/<app>/<base-domain>/bootstrap.json` in an encrypted secret store or password manager vault.

❌ INCORRECT: paste the mnemonic into a shared Slack channel for “safe keeping”.

## AWS hardening basics

- Use least-privilege IAM for humans and CI (separate deploy role from read-only).
- Keep Route53 + ACM validation in the same account as the deployment where possible.
- Rate limiting is enabled by default; use `DISABLE_RATE_LIMITING` / `DISABLE_FEDERATION_RATE_LIMITING` only for controlled debugging (see `docs/configuration.md`).
- AWS WAF is not provisioned by default; add it in CDK if you need WAF-level protections.

## Tenant and instance isolation

Lesser is designed to support multi-tenant deployments. If you run multiple instances:

- Keep per-tenant domains and credentials isolated.
- Avoid cross-tenant access in operational tooling (logs/metrics).
- Use separate AWS accounts if you need hard isolation boundaries.

## Incident response starting points

- Tail API logs:

  ```bash
  ./lesser logs --app <app> --function api --env dev --aws-profile <profile>
  ```

- Runbook: `docs/operations/runbook.md`
