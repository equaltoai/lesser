# Lesser Deployment Guide (AWS CDK)

Lesser deployments are managed with AWS CDK (CloudFormation) under `infra/cdk/`.
For environment-specific deploys, prefer the Makefile targets in the repo root.

## Prerequisites

- AWS CLI configured (`aws configure`)
- AWS CDK v2 installed (`npm install -g aws-cdk`)
- Go 1.25 or later (builds Lambda zips)
- `make`
- Docker (optional; only needed for local DynamoDB via `make local-dynamodb`)

## Build Artifacts

Artifacts land in `bin/`.

```bash
# Build Lambda zips (incremental)
make build-lambdas

# Force rebuild Lambda zips
make rebuild-lambdas

# Full rebuild (Lambda zips + cloudfront-keygen + auth-ui + go build ./...)
make build
```

## One-Time Setup

### 1) Bootstrap CDK

```bash
cd infra/cdk
cdk bootstrap aws://YOUR-ACCOUNT-ID/us-east-1
```

### 2) Deploy shared resources (once per AWS account)

Creates shared KMS + Secrets Manager resources used by all environments.

```bash
make deploy-shared AWS_PROFILE=YOUR_PROFILE
```

## Deploy an Environment

Deployment is driven by Make targets, which build artifacts and then deploy the relevant CDK stacks.

### Development

```bash
make deploy-dev AWS_PROFILE=YOUR_PROFILE
```

### Staging

```bash
make deploy-test DOMAIN=staging.yourdomain.com AWS_PROFILE=YOUR_PROFILE
```

### Production

```bash
make deploy-live DOMAIN=yourdomain.com AWS_PROFILE=YOUR_PROFILE
```

## Configuration

Environment configuration lives in:
- `infra/cdk/config/development.yaml`
- `infra/cdk/config/staging.yaml`
- `infra/cdk/config/production.yaml`

You can also deploy manually with CDK context:

```bash
cd infra/cdk
cdk deploy --all --context environment=production --context domain=yourdomain.com --require-approval broadening
```

## Rollback

Preferred: redeploy a known-good git revision.

```bash
git checkout <known-good-sha>
make rebuild-lambdas
cd infra/cdk && cdk deploy --all --context environment=production --require-approval broadening
```

## References

- `docs/deployment.md` (quick deployment guide)
- `infra/cdk/README.md` (CDK stack details)
