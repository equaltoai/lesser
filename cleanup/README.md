# Git history cleanup assets

`filter-repo-drop-paths.txt` lists the pathspecs to strip from history:
- Go build caches and other transient directories (`.gocache/`, `.gomodcache/`, `bin/`, `build/`, `cdk.out/`, `tmp/`, `temp/`, `scripts/node_modules/`, bootstrap snapshots).
- Large lambda bundles committed either at the repository root or as compiled binaries under `cmd/**` (e.g. `activity-processor`, `api`, `cmd/api/api`, `federation-aggregator`, `graphql`, etc.).

`replace-secrets.txt` is passed to `git filter-repo --replace-text` to blank historical test credentials (the CloudFront PEM blob and an AWS sample key that trips scanners).

Run the rewrite from a disposable clone:

```bash
git filter-repo --paths-from-file cleanup/filter-repo-drop-paths.txt --invert-paths --force
git filter-repo --replace-text cleanup/replace-secrets.txt --force
```

Afterward re-run `git filter-repo --analyze`, rebuild (`./lesser build lambdas`), and run the tests before coordinating the force-push with the team.
