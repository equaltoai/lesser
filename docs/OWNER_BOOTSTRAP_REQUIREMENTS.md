## Owner Bootstrap Requirements

This document captures the non-negotiable requirements for provisioning the canonical
`admin` account during deployments. The intent is to have a **deterministic, automated,
and idempotent** process that requires zero manual steps beyond retrieving the wallet
secret when a human needs to log in.

### 1. Goals

1. Provision exactly one privileged account (`admin`) in every environment (`dev`,
   `staging`, `production`) the first time the DynamoDB table is created.
2. Seed all dependent artifacts (ActivityPub actor, wallet credential/index, OAuth
   client, secrets) so the admin can authenticate solely via wallet signatures.
3. Keep the process idempotent; rerunning deploys must **not** rotate credentials or
   insert duplicates. A rerun should be a no-op unless a force flag is provided.
4. Integrate with existing infrastructure tooling (CDK + `make deploy-*`) so the flow
   runs automatically as part of the stack deployment, with clean, concise logging.

### 2. Artifacts To Seed

| Artifact                       | Description                                                                             |
| ----------------------------- | --------------------------------------------------------------------------------------- |
| `USER#admin / METADATA`       | User row with `Role=admin`, `Approved=true`, wallet-only recovery methods, camelCase GSIs|
| `ACTOR#admin / PROFILE`       | ActivityPub actor document with RSA keys, endpoints, numeric ID, etc.                   |
| Wallet credential             | `PK=USER#admin, SK=WALLET#<address>` row containing normalized address + timestamps.    |
| Wallet index                  | `PK=WALLET#ethereum#<address>, SK=USER#admin` to support reverse lookups.               |
| OAuth client                  | `PK=OAUTH_CLIENT#<clientID>, SK=CLIENT` with `gsi1PK=OWNER#admin` and camelCase attrs.  |
| Secrets                       | `lesser/<env>/admin-wallet` + `lesser/<env>/admin-oauth` in Secrets Manager.            |
| Optional preferences          | Any required defaults (quote permissions, visibility, etc.) inserted via the same flow. |

### 3. Secrets Management

1. Generate the admin wallet (Ethereum) and RSA key pair **once** per environment.
2. Store wallet material in Secrets Manager (`lesser/<env>/admin-wallet`) as JSON:
   ```json
   {
     "address": "0x...",
     "private_key": "0x...",
     "chain_id": 1,
     "wallet_type": "ethereum",
     "username": "admin",
     "created_at": "ISO8601"
   }
   ```
3. Store the canonical OAuth client in `lesser/<env>/admin-oauth`:
   ```json
   {
     "client_id": "…",
     "client_secret": "…",
     "redirect_uris": [
       "https://<domain>/auth/callback",
       "urn:ietf:wg:oauth:2.0:oob"
     ],
     "name": "Owner Console",
     "username": "admin",
     "created_at": "ISO8601"
   }
   ```
4. Secrets must be created/updated via AWS SDK (Go or Lambda custom resource),
   not shell commands. Every operation should be logged and emit metrics.

### 4. Idempotent Writes

* Use DynamoDB `TransactWriteItems` (or conditional `PutItem`) for all rows.
* Every write includes `ConditionExpression attribute_not_exists(PK)` to ensure we do
  not overwrite existing data. If a row already exists, the transaction must **fail**
  loudly so the deploy stops instead of silently diverging.
* Before attempting the transaction, query `USER#admin / METADATA`.
  - If it exists, skip the entire bootstrap (no secrets rotated).
  - If it does not, run the transaction and create the secrets.

### 5. Deployment Integration

1. Package the bootstrap logic as a Go binary or a CDK custom resource Lambda.
   Requirements:
   - Inputs: environment name, table name, domain, secret names.
   - Outputs: success/failure plus structured logs.
   - Permissions: DynamoDB table (read/write) + Secrets Manager (read/write) scoped
     exactly to the relevant resources.
2. Wire the binary/custom resource into `make deploy-*` or the CDK stacks so it runs
   automatically after the main table is deployed and before we consider the deploy
   successful.
3. Console/log output must be concise:
   ```
   [owner-bootstrap] checking USER#admin … not found
   [owner-bootstrap] provisioning admin artifacts …
   [owner-bootstrap] wrote wallet secret lesser/development/admin-wallet
   [owner-bootstrap] bootstrap complete
   ```
   No manual “next steps” text should appear during automated runs.

### 6. Observability & Failure Handling

* Emit structured logs (JSON) for every step: existence checks, DynamoDB writes,
  secret creation, and completion.
* Surface metrics (CloudWatch or custom) for success/failure counts.
* On failure, the deploy must abort with a clear error message (e.g., “admin bootstrap
  failed: wallet secret exists but USER#admin is missing”). Never leave the system
  in a partially seeded state.

### 7. Manual Access Workflow

1. Retrieve the wallet secret from Secrets Manager using AWS CLI (with MFA/SSO):
   ```bash
   aws secretsmanager get-secret-value \
     --secret-id lesser/<env>/admin-wallet \
     --query SecretString --output text
   ```
2. Import the `private_key` into MetaMask (Import account → Private key).
3. Log in via the wallet challenge/signature flow as username `admin`.
4. The OAuth client secret is available in `lesser/<env>/admin-oauth` for console apps.

### 8. Cleanup of Previous Hacks

* Remove any `make bootstrap-owner` target, shell scripts, or node generators that
  attempt to seed the admin row manually.
* Delete instructions printed to stdout during deploys.
* Ensure no temporary directories (`tmp/owner_admin_seed`, `wallet_secret.json`, etc.)
  are created as part of the automated pipeline.

Following this document should result in a single, deterministic provisioning
mechanism that can be audited, monitored, and confidently executed in production.
