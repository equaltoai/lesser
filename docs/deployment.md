# Deployment (Quick Start)

For most operators, deployment is done via the `lesser` CLI (`lesser up`) instead of invoking CDK directly.

See `docs/DEPLOYMENT_GUIDE.md` for the full operator workflow; this file is a short reference.

## Prerequisites

- AWS CLI configured (and logged in for your chosen profile)
- AWS CDK v2 installed and on `PATH` (`npm install -g aws-cdk`)
- Go 1.25+
- A public Route53 hosted zone that exactly matches your `base-domain` (for example: `example.com`)
- An AWS profile with a default region configured (the CLI derives region from the profile)

## Deploy

Build the CLI:

```bash
go build -o lesser ./cmd/lesser
```

Deploy **dev + live** (and optionally **staging**):

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny
```

Bootstrap wallet key material:

- `lesser up` prints a 24-word Ethereum mnemonic **once** when it is generated.
- Pass `--out <path>` to also write it to disk (the file is created with `0600` permissions).

Local receipt (non-secret):

- `~/.lesser/<app>/<base-domain>/state.json`

## Verify “locked but reachable”

```bash
# Lock state + bootstrap actor descriptor
curl -s https://api.dev.example.com/setup/status | jq .

# Empty timeline while locked
curl -s https://api.dev.example.com/api/v1/timelines/public | jq .
```

## Activation

The setup wizard UI is out of scope for this repo work; the backend contract lives under `https://api.<stage-domain>/setup/*` and will be consumed by a separate Auth UI project.

### Build Failures
```bash
# Ensure correct architecture
GOOS=linux GOARCH=arm64 go build
```

### Certificate Issues
- Ensure certificate is in us-east-1 for CloudFront
- Verify DNS validation records are in place

### Domain Not Working
- Check Route53 hosted zone
- Verify CloudFront distribution status
- Allow up to 15 minutes for propagation

## Next Steps

- [Configuration Reference](configuration.md) - Customize your instance
- [Monitoring Guide](monitoring.md) - Set up comprehensive monitoring
- [Federation Guide](federation.md) - Connect to the Fediverse
