# CMS Blog Launch Runbook

This runbook is for the first Theory Cloud Blog/CMS launch on a Lesser instance. It covers the MCP-first,
fediverse-first MVP only: canonical Article publishing, server-side rendering/sanitization, draft review,
federation readback, and operational rollback.

## Preconditions

- Deploy through the canonical path:

  ```bash
  go build -o lesser ./cmd/lesser
  ./lesser build lambdas
  ./lesser up --app <app> --base-domain <domain> --aws-profile <profile> --stage dev
  ```

- The instance may be locked immediately after deploy. Confirm and unlock through setup before publish probes:

  ```bash
  curl -s "https://dev.<domain>/setup/status" | jq .
  ```

- Enable only the CMS gates required for the MVP: long-form CMS, drafts, revisions if used, and scheduled
  publishing only when explicitly exercised.
- Do not expand launch validation into rich editor, Simulacrum retrofit, newsletter, comments, search, RSS,
  timeline/trending, or general document hosting. Those remain outside Project 36's cutline.

## Storage limits and body strategy

The MVP stores Article source in DynamoDB as the Article/Draft `content` field. There is **no S3 body offload**
for the launch path.

Current deterministic limits:

- Article source: **256 KiB** before rendering (`cmsrender.MaxArticleSourceBytes`).
- Rendered/sanitized HTML: **512 KiB** after rendering (`cmsrender.MaxArticleRenderedHTMLBytes`).
- Supported source formats: Markdown and HTML-as-sanitizer-input.
- Unsupported formats, invalid UTF-8, source-size overflow, and rendered-size overflow fail deterministically.
- Lesser never silently truncates source or rendered output.

Escalate a follow-up body-storage design only if real launch drafts need more than 256 KiB of source or
operators observe DynamoDB item-size pressure from Article metadata/media growth.

## Legacy `/objects/<uuid>` dry-run and alias policy

Existing Article objects under `https://<domain>/objects/<uuid>` remain their own ActivityPub object identity.
The launch path must not rewrite them into `/articles/<slug>` IDs.

Dry-run policy:

1. Identify local Article rows whose stored ID is `https://<domain>/objects/<uuid>` and whose Article slug is non-empty.
2. Propose the non-authoritative browser alias `https://<domain>/articles/<slug>`.
3. Keep `proposedCanonicalID` equal to the stored legacy `/objects/<uuid>` ID.
4. Report conflicts before any alias/backfill write:
   - more than one legacy Article proposes the same tenant+slug alias;
   - a canonical `https://<domain>/articles/<slug>` Article already occupies that tenant+slug.
5. Do not introduce duplicate canonical ActivityPub IDs.

The dry-run planner is `cms.PlanLegacyArticleMigration`. It is pure/read-only and should be wired to operator
tooling before any apply-capable alias migration exists.

## Publish probe

Use the authenticated GraphQL path so the probe exercises the same authoring contract used by MCP consumers:

1. Create an Article draft.
2. Call `draftPreview(id: ID!)` and confirm:
   - `success: true`;
   - `renderedHtml` is sanitized server output;
   - `sourceBytes` and `renderedBytes` are present.
3. Publish the draft.
4. Confirm the returned `Article.id` is:

   ```text
   https://<domain>/articles/<slug>
   ```

5. Confirm the slug is immutable on post-publish update; title/body/metadata updates should preserve the same ID.

## Federation fetch probe

Fetch the published Article as a remote peer would:

```bash
curl -i \
  -H 'Accept: application/activity+json' \
  "https://<domain>/articles/<slug>"
```

Expected:

- HTTP 200 for a live Article.
- `type` is `Article`.
- `id` is exactly `https://<domain>/articles/<slug>`.
- `content` is sanitized HTML from lesser's renderer.
- `bto` and `bcc` are absent.

For a deleted Article, expect HTTP 410 with an ActivityPub Tombstone for the same Article ID.

## UI smoke

In a browser:

1. Open `https://<domain>/articles/<slug>`.
2. Confirm the article body matches the GraphQL draft preview semantically.
3. Confirm unsafe HTML is not rendered as executable content.
4. Confirm public chrome/templates do not reveal draft-only fields, raw source, private tokens, or hidden recipients.

If GraphQL is used for the smoke path, `articleBySlug(slug: "<slug>")` must return the same `id` as the
browser and ActivityPub fetch path.

## Observability checks

Article renderer and federation failures are normal ops-path signals, not optional ad hoc logs.

CloudWatch Logs message filters:

```text
cms_article_render_failure
cms_draft_render_failure
cms_article_federation_attempt
cms_article_federation_outcome
```

CloudWatch EMF namespace:

```text
Lesser/CMS
```

Critical metrics:

- `ArticleRenderFailure` with dimensions `Stage=<dev|staging|live>, Status=failure`.
- `ArticleFederationFailure` with dimensions `Stage=<dev|staging|live>, Status=failure`.

The per-stage critical alarm stack creates alarms for both metrics and sends them to the critical alert topic.
Treat a single production failure as actionable during launch soak: inspect the structured fields
`Operation`, `article_id` or `draft_id`, `failure_stage`, `activity_type`, `error_kind`, and `source_bytes`.
Logs intentionally do **not** include raw draft/article content.

## Rollback and residuals

Rollback code through the normal release path:

1. Revert the offending commit or check out the prior reviewed release.
2. Rebuild and redeploy with `./lesser up` through dev, staging if used, then live.
3. Preserve the bootstrap mnemonic at `~/.lesser/<app>/<domain>/bootstrap.json`.

Known residuals:

- ActivityPub activities already delivered to remote peers cannot be recalled by rollback.
- Legacy `/objects/<uuid>` Articles remain canonical legacy objects; `/articles/<slug>` aliases are planned by dry-run
  and are not applied in the launch path.
- RSS/Atom, comments, newsletter delivery, rich editor, Simulacrum retrofit, timeline/trending coupling, and general
  document hosting remain non-goals.
- If sanitized output removes author-intended markup, adjust the renderer policy in a reviewed follow-up rather than
  bypassing the sanitizer.
