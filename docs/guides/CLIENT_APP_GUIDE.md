# Lesser FaceTheory Client Guide

This guide describes the authoritative client path for Lesser-managed deployments:

- Lesser owns the stage apex domain and `/l/` routing.
- FaceTheory owns the app contract and SSR behavior.
- Normal client updates use `lesser client install`, not `lesser up`.

## Summary

`lesser up` is now responsible for one-time provisioning of the shared client host resources for each stage:

- CloudFront routing on the stage apex domain
- `/l` and `/l/*` routed to the Lesser-managed SSR host
- `/l/_assets/*` routed to the public client asset bucket
- `/auth` and `/auth/*` routed to the auth UI bucket
- `/auth/wallet/*` routed back to the API origin
- a private, versioned client artifact bucket that stores install history
- an active install manifest key (`install/current.json`) that tells the SSR host which release is live

After that, ordinary client releases should use `lesser client install` from the FaceTheory app repo. That flow updates
artifacts and the active manifest only. It does not require re-running the Lesser stack or re-provisioning lesser-body.

## Routing model

Lesser stage stacks reserve the following top-level paths:

- `/api/*`, `/.well-known/*`, `/setup/*`, `/media/*`: API/system traffic
- `/auth` and `/auth/*`: static auth UI
- `/auth/wallet/*`: API bypass route for wallet flows
- `/l` and `/l/*`: FaceTheory SSR host
- `/l/_assets/*`: static client assets

Important behavior details:

- `GET /` is still handled by the API and redirects to `/l/`.
- CloudFront no longer performs SPA fallback for `/l/*`.
- The CloudFront rewrite function only normalizes `/l` to `/l/` and applies directory-index semantics for `/auth/*`.
- CSP for the FaceTheory app should come from the origin response. CloudFront adds baseline security headers but does
  not inject a competing CSP on `/l` routes.

## FaceTheory install contract

Every FaceTheory app repo that will be published through Lesser should include a `facetheory.lesser.json` file at the
repo root (or pass it explicitly via `--config`).

Example:

```json
{
  "schema_version": 1,
  "app_name": "my-client",
  "display_name": "My Client",
  "version": "0.1.0",
  "build": {
    "command": ["pnpm", "build"]
  },
  "server": {
    "dir": "build/server",
    "entry": "handler.mjs",
    "export": "handler"
  },
  "assets": {
    "dir": "build/client"
  }
}
```

Contract requirements:

- `schema_version` must currently be `1`.
- `package.json` must include `@theory-cloud/facetheory`; the current recommended release pin is:

  ```json
  {
    "dependencies": {
      "@theory-cloud/facetheory": "https://github.com/theory-cloud/FaceTheory/releases/download/v4.0.1/theory-cloud-facetheory-4.0.1.tgz"
    }
  }
  ```

- `lesser client install` validates that the dependency is present; it does not require this exact URL so local workspace
  development remains possible. For operator releases, use the pinned release asset (or an explicitly reviewed newer
  FaceTheory release) so the client artifact has clear provenance.
- `server.dir` must contain the built SSR bundle.
- `server.entry` must point at a file inside `server.dir`.
- `server.export` defaults to `handler` when omitted.
- `assets.dir` must contain the public browser assets.
- Asset URLs must resolve under `/l/_assets/...`.

The Lesser SSR host imports the installed server bundle in-process and invokes the configured export. The exported
handler may return a standard `Response` or a Lambda-style response object.

## Install workflow

Prerequisites:

- `lesser up` has already been run for the app and base domain.
- The local deployment receipt exists at `~/.lesser/<app>/<base-domain>/state.json` or is passed with `--state`.
- The FaceTheory app is checked out locally.

Recommended command:

```bash
./lesser client install \
  --app <slug> \
  --base-domain <example.com> \
  --aws-profile <profile> \
  --config ./facetheory.lesser.json
```

Useful flags:

- `--stage dev|staging|live|both|all`
- `--skip-build`
- `--state <path>`

What the command does:

1. Validates the FaceTheory app contract.
2. Installs app dependencies if needed and runs the configured build command unless `--skip-build` is set.
3. Uploads the SSR server bundle to the private artifact bucket under `installs/<install-id>/server/...`.
4. Uploads browser assets to the public client bucket under `l/_assets/...`.
5. Writes an immutable history manifest at `installs/<install-id>/manifest.json`.
6. Flips the active manifest key (normally `install/current.json`) to the new manifest.
7. Invalidates CloudFront for `/l` and `/l/*`.
8. Records the active install in the local Lesser receipt.

## Receipt and stack outputs

The stage receipt now carries the values the install flow needs:

- `ClientBucketName`
- `ClientArtifactBucketName`
- `ClientInstallManifestKey`
- `FrontendDistributionId`

After an install, Lesser also records the active release under:

- `stages.<stage>.client_install.app_name`
- `stages.<stage>.client_install.version`
- `stages.<stage>.client_install.install_id`
- `stages.<stage>.client_install.manifest_key`
- `stages.<stage>.client_install.server_root`
- `stages.<stage>.client_install.assets_root`

## Verification checklist

Verify a release after `lesser client install`:

- `GET https://dev.<base-domain>/l/` returns SSR HTML from the new build.
- Deep routes such as `https://dev.<base-domain>/l/@alice` render successfully without SPA fallback assumptions.
- Browser assets load from `/l/_assets/...`.
- `GET https://dev.<base-domain>/auth/login` still resolves through auth directory-index routing.
- `/auth/wallet/*` traffic still reaches the API origin instead of the auth bucket.
- The active manifest in the artifact bucket references the expected `install_id`.

Useful checks:

```bash
curl -i https://dev.<base-domain>/l/
curl -i https://dev.<base-domain>/l/_assets/<asset-name>
curl -i https://dev.<base-domain>/auth/login
AWS_PROFILE=<profile> aws s3 cp \
  s3://<artifact-bucket>/install/current.json -
```

## Rollback

Preferred rollback:

- Check out the last known good FaceTheory app revision.
- Re-run `lesser client install` from that revision.

Hot rollback using the existing artifact history:

1. Identify the previous good install ID in the artifact bucket under `installs/<install-id>/manifest.json`.
2. Copy that manifest back to the active manifest key.
3. Invalidate `/l` and `/l/*` on the stage CloudFront distribution.

Example:

```bash
AWS_PROFILE=<profile> aws s3 cp \
  s3://<artifact-bucket>/installs/<install-id>/manifest.json \
  s3://<artifact-bucket>/install/current.json \
  --content-type 'application/json; charset=utf-8' \
  --cache-control 'no-store'
```

## Operational invariants

These are the important rules to preserve:

- Do not run `lesser up` for ordinary client releases.
- Do not depend on SPA fallback behavior under `/l/*`.
- Do not publish client browser assets to the bucket root; they belong under `l/_assets/`.
- Do not delete install history during routine client updates.
- Keep lesser-body provisioned by the stack; client releases should not have to toggle or recreate it.
