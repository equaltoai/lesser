# Managed Provisioning Roadmap (lesser)

Purpose: align the `lesser` CLI and instance bootstrapping with the managed provisioning flow in `lesser-host`.

Goals:
- Support single-stage deploy for managed provisioning.
- Seed initial admin using a public wallet address (no mnemonic, no wallet secret).
- Unlock instance immediately after provisioning.
- Keep passkey setup optional and recommended after wallet login.

Non-goals:
- Changing wallet auth or passkey architecture beyond setup flow adjustments.
- Multi-stage managed deployments in one request.

## Reserved Admin Wallets
The following wallet addresses must never be used as instance admin wallets:
- `0x80189edb676d51b2fb2257b2ad38e018b20ca46e` (lesser.host admin wallet)
- `0x1e14865a53a994b01b9ccfef42669dc0bfe98805` (Safe + 1% recipient, `TipSplitter.lesserWallet`)

## Runner Input Contract (schema=1)
Managed provisioning uses a JSON input file consumed by both `./lesser up` and `./lesser init-admin` via
`--provisioning-input`.

Example:
```json
{
  "schema": 1,
  "slug": "my-instance",
  "stage": "dev",
  "admin_wallet_address": "0x4444444444444444444444444444444444444444",
  "admin_username": "my-instance",
  "admin_wallet_chain_id": 1,
  "lesser_host_url": "https://lab.lesser.host",
  "lesser_host_attestations_url": "https://lab.lesser.host",
  "lesser_host_instance_key_arn": "arn:aws:secretsmanager:us-east-1:123456789012:secret:instanceKey",
  "translation_enabled": false,
  "consent_message": "lesser.host requests your consent to provision a managed instance...\n",
  "consent_signature": "0x..."
}
```

Notes:
- `admin_username` defaults to `slug` when omitted.
- `stage` supports `dev|staging|live` (managed runners typically use `dev` and `live`).
- `admin_wallet_chain_id` overrides `--chain-id` for `init-admin` when supplied.
- `lesser_host_*` and `translation_enabled` are optional integration config passed through `./lesser up` to the CDK deploy.
- `consent_message` and `consent_signature` can satisfy `init-admin` without extra flags.
- `--aws-profile` is optional when AWS ambient credentials are available.

## Runner Commands

1. Deploy a single stage without generating a bootstrap mnemonic (managed provisioning):
```bash
./lesser up --base-domain example.com [--aws-profile <profile>] --provisioning-input provision.json
```

`admin_wallet_address` is mapped to the deployment bootstrap wallet address so the deploy does not emit or require a
mnemonic.

2. Seed the initial admin (wallet-only) and unlock the instance:
```bash
./lesser init-admin --base-domain example.com [--aws-profile <profile>] --provisioning-input provision.json \\
  --signature <0x...> --message-file <path> [--chain-id 1] [--reserved-wallets <csv>]
```

`init-admin` verifies the EIP-191 `personal_sign` signature and then:
- creates/ensures the admin user + actor + wallet credential + wallet index
- updates instance state: `locked=false`, `primaryAdminUsername=<admin_username>`, clears `bootstrapWalletAddress`, and
  sets `activatedAt` (if missing)

## Milestones

**Milestone 1: Single-Stage Deploy Support**
- Add a `--stage` flag to `./lesser up` to deploy only `dev`, `staging`, or `live`.
- Keep existing default behavior (dev+live) for non-managed workflows.
- Ensure outputs and receipts reflect only the deployed stage.

**Milestone 2: Admin Seed Command (Wallet-Only)**
- Add a new CLI command: `./lesser init-admin`.
- Inputs: `--username`, `--wallet-address`, `--chain-id`, `--kms-key-id` (+ standard deploy flags).
- Create/ensure admin user, actor, wallet credential, wallet index.
- Set instance state to `locked=false` and `primaryAdminUsername`.
- Do not store or create any wallet private key.
- Reject reserved wallet addresses (built-ins + runner-provided list).

**Milestone 3: Runner Integration**
- Runner executes `./lesser up --stage <dev|live> --provisioning-input provision.json`.
- Runner executes `./lesser init-admin --provisioning-input provision.json ...`.
- Ensure idempotency so reruns do not corrupt data.
- Runner passes a reserved wallet list to `init-admin` for defensive validation (optional).

**Milestone 4: Passkey-Only Setup UX**
- When instance is already activated, show setup UI focused on (optional) passkey enrollment.
- Ensure wallet login works immediately without requiring the legacy bootstrap flow.

**Milestone 5: Tests and Docs**
- Add tests for single-stage deploy.
- Add tests for admin seeding with a public wallet.
- Document managed provisioning flow alignment.

## Data Writes Required (Init-Admin)
- User record (admin role, approved, unlocked).
- Actor record + encrypted private key (KMS).
- Wallet credential + wallet index.
- Instance state update: `locked=false`, `primaryAdminUsername` set, bootstrap wallet cleared.

## Acceptance Criteria
- Managed provisioning can deploy only dev or only live.
- Admin wallet can log in immediately after provisioning.
- No bootstrap mnemonic is created for managed instances.
- Passkey setup is optional and available after login.
- Reserved wallets are rejected when supplied to `init-admin`.

## Open Questions
- Should `init-admin` require confirmation if an admin already exists?
- Should `init-admin` allow updating the admin wallet if the wallet is already linked?
- Should the setup wizard be accessible after activation strictly for passkey enrollment?
