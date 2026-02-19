# Coordinating with `greater-components`: contract pin + tag workflow

Whenever Lesser’s GraphQL contract changes—especially for DM v1’s new inbox/requests fields—the Greater UI must pin those artifacts before its adapters are regenerated and published. The canonical instructions live in `greater-components/AGENTS.md`, so this doc summarizes the DM-specific checklist and highlights the required **tag → pin → regenerate** path.

## When to update

1. DM v1 schema updates land in `lesser` (M0) and the `docs/contracts/graphql-schema.graphql` file changes.
2. A release tag (e.g., `vX.Y.Z`) is cut that contains the DM schema. Prefer a release tag over a moving branch to keep `greater-components` reproducible.

## Tag & pin workflow

1. From `greater-components`, select the tag that shipped the new DM schema:
   ```bash
   LESSER_TAG="$(git -C ../lesser tag --sort=-v:refname | head -n 1)"
   LESSER_COMMIT="$(git -C ../lesser rev-parse "${LESSER_TAG}")"
   ```
2. Copy Lesser’s published contracts into the mirrored docs directory for adapters:
   ```bash
   git -C ../lesser show "${LESSER_TAG}:docs/contracts/openapi.yaml" \
     > docs/lesser/contracts/openapi.yaml
   git -C ../lesser show "${LESSER_TAG}:docs/contracts/graphql-schema.graphql" \
     > docs/lesser/contracts/graphql-schema.graphql
   ```
3. Update `docs/lesser/contracts/LESSER_REF.txt` with the tag/commit:
   ```bash
   printf "tag: %s\ncommit: %s\n" "${LESSER_TAG}" "${LESSER_COMMIT}" \
     > docs/lesser/contracts/LESSER_REF.txt
   ```
4. Regenerate derived artifacts so the GraphQL adapters and CLI see the new schema:
   ```bash
   corepack pnpm generate:openapi
   corepack pnpm generate:graphql
   corepack pnpm generate-registry
   ```
5. Run the adapter typechecks (per `greater-components/AGENTS.md`):
   ```bash
   corepack pnpm --filter @equaltoai/greater-components-adapters typecheck
   corepack pnpm --filter @equaltoai/greater-components-social typecheck
   ```
6. Commit the pinned contracts + generated output in `greater-components`, referencing the Lesser tag in the message (e.g., `chore: pin lesser v1.2.0 schema`).

## Additional DM v1 notes

- Mention in the commit/PR that this pin unlocks the DM inbox/requests experience described in `docs/planning/dm-v1-graphql-schema-diff.md`.
- If the DM schema changes require new queries/mutations, ensure the generated adapters support them before releasing (update `codegen.ts` if needed).

## References

- `greater-components/AGENTS.md` (contract pin + generation commands)
- `docs/contracts/README.md` (contract lifecycle)
- `docs/planning/dm-v1-graphql-schema-diff.md` (expected GraphQL diff for DM v1)
