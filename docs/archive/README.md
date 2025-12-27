# Docs Archive

This directory contains **archived** documents that are kept for historical context (past plans, investigations, and snapshots).

Archived docs are **not required to stay current** with the codebase and may contain outdated counts, paths, or implementation details.

## Organization

- `docs/archive/planning/` — old planning notes and design explorations
- `docs/archive/specs/` — completed/obsolete specs kept for reference
- `docs/archive/notes/` — incident reports, one-off summaries, and session notes

## Policy

- Prefer `git mv` when archiving so history is preserved.
- Avoid linking to archived docs from primary docs; if needed, link with an explicit “archived” label.
- CI doc drift checks should ignore `docs/archive/` (by design).

