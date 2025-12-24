# Lesser Deployment Guide (`lesser up`)

Lesser deployments are managed with AWS CDK (CloudFormation) under `infra/cdk/`, but the operator interface is the `lesser` CLI.

`lesser up` deploys a new instance into a **locked-but-reachable** state, always deploying **dev** and **live** (and optionally **staging**).

## Prerequisites

- AWS CLI configured (including `aws sso login --profile ...` if you use SSO)
- AWS CDK v2 installed and on `PATH` (`npm install -g aws-cdk`)
- Go 1.25+ (builds Lambda zips + runs CDK app)
- A public Route53 hosted zone that exactly matches your `base-domain` (the CLI will fail fast if it can’t find it)

## Deploy

Build the CLI:

```bash
go build -o lesser ./cmd/lesser
```

Deploy dev + live:

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny
```

Optionally deploy staging too:

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --with-staging
```

Bootstrap wallet key material:

- `lesser up` prints a 24-word Ethereum mnemonic **once** when it is generated.
- To also write it to disk, pass `--out <path>` (the file is created with `0600` permissions).

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --out ~/.lesser/my-lesser/example.com/bootstrap.json
```

## What Gets Deployed

Stacks:

- Shared stack (once per app/account/region): `<app>-shared`
- Stage stacks: `<app>-dev`, `<app>-live` (and `<app>-staging` if enabled)

Local receipt (non-secret):

- `~/.lesser/<app>/<base-domain>/state.json`

Bootstrap state:

- Each stage’s DynamoDB table gets an `InstanceState` record set to locked, with the bootstrap wallet address recorded.

## Locked Instance Behavior (Pre-activation)

While locked:

- Timelines and list endpoints return empty collections.
- Signup and publishing endpoints return `403`.
- `/.well-known/webfinger` returns only the bootstrap actor; other users return `404`.
- Federation requests for the bootstrap actor return `403`.
- NodeInfo behaves normally, but registrations are not open.

Quick checks:

```bash
# Lock state + bootstrap actor descriptor
curl -s https://api.dev.example.com/setup/status | jq .

# Empty timeline while locked
curl -s https://api.dev.example.com/api/v1/timelines/public | jq .
```

## Activation

The setup wizard UI is out of scope for this repo work; the backend contract lives under `https://api.<stage-domain>/setup/*` and will be consumed by a separate Auth UI project.

Activation, when implemented in the wizard:

- Bootstrap user logs in via “connect wallet + sign challenge”.
- Wizard creates a real admin (passkey and/or wallet) and logs in as that admin.
- Wizard finalizes activation (deletes bootstrap actor/account and enables liveness).

## References

- `docs/specs/LESSER_UP_CLI.md` (requirements + stack naming)
- `docs/deployment.md` (quick start)
- `infra/cdk/README.md` (CDK details)
