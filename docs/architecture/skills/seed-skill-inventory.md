# Repo-backed SKILL.md seed inventory

Status: Project 21 M3 seed inventory (#699), generated from the Lesser repo on 2026-05-13.

## Authority boundary

This inventory is source material only. Local `SKILL.md` files are useful seeds for canonical `Skill`, `SkillRevision`, and `SkillProposal` records, but they are not the system of record. Canonical authority lives in Lesser after an explicit import/proposal/approval flow.

## Summary

- Files found: 26
- Unique skill names: 14
- Duplicate digest groups: 12 `.claude`/`.codex` pairs are byte-for-byte copies.
- Unique repo-root-only skill: `skills/high-risk-domain-planning/SKILL.md`
- Claude-only direct authorized email skill: `.claude/skills/check-authorized-email/SKILL.md`

## Inventory

| Path | Skill name | SHA-256 | Bytes | Seed role | Publishability gaps |
|---|---|---|---:|---|---|
| `.claude/skills/check-authorized-email/SKILL.md` | `check-authorized-email` | `a230c458652a30cbd84e157a4085076e5dc18dd3b6f0aea6d6a5526b22f2dac3` | 7693 | Claude-only local authorized-email seed; private review required | manifest, approval, exposure, capability policy, private routing |
| `.claude/skills/coordinate-framework-feedback/SKILL.md` | `coordinate-framework-feedback` | `9b1b9fd9ebba18303bf4ddf5f4bb29de350ed58b07272e05b92569969983c548` | 10249 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/create-github-project/SKILL.md` | `create-github-project` | `8912e838cd1731231d221202f68942d14af7dd77c26f203cb58a8bc6300a4a5c` | 9704 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/deploy-instance/SKILL.md` | `deploy-instance` | `a8cba5eab37b24a63b4bd64df2b46c1f23e393c2eedec7a0b983e0a504ea6a40` | 13354 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/enumerate-changes/SKILL.md` | `enumerate-changes` | `841b80fd7c38495936e54056041bb25206fa5c3e33fb672a0e089771c025ceb3` | 10825 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/implement-milestone/SKILL.md` | `implement-milestone` | `641a273b7aa051e8971cb1873b2c7bf5d89f4acf935f8cf706dbaa2c3474661d` | 9761 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/investigate-issue/SKILL.md` | `investigate-issue` | `227f8a4f590e58db97e39170ec9eede597f5f369aee1074cbce6ec38409b0205` | 9693 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/plan-roadmap/SKILL.md` | `plan-roadmap` | `24de5037d629765a367d14b73605ca9329518da5227d4162c996bd85fc26b7cd` | 10730 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/preserve-mastodon-api-compat/SKILL.md` | `preserve-mastodon-api-compat` | `b5081eb525243ab3d393ab0122dde6f6d87b97baebef9d40bf35726a516f6c46` | 13403 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/protect-federation-trust/SKILL.md` | `protect-federation-trust` | `69ae488636fc4bd8d4ce587a1dbcc2a5c866dd01dde7924209f584fa7664f7db` | 13143 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/review-advisor-brief/SKILL.md` | `review-advisor-brief` | `9be0b1877c959f1687d59b687e8449ce6c3692572328861d95d374cf33c750bf` | 9940 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/scope-need/SKILL.md` | `scope-need` | `5264637e109a0b312354f0e07da8900137832eae87956aecc3b6c150a7411e46` | 11220 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.claude/skills/validate-schema/SKILL.md` | `validate-schema` | `9a1399a0b863a32a31c4dc6bb2bfc3e55b4e62f4f02bd5ccef25ef87da688d25` | 14040 | duplicate Claude-runtime seed; retain as provenance only | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/coordinate-framework-feedback/SKILL.md` | `coordinate-framework-feedback` | `9b1b9fd9ebba18303bf4ddf5f4bb29de350ed58b07272e05b92569969983c548` | 10249 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/create-github-project/SKILL.md` | `create-github-project` | `8912e838cd1731231d221202f68942d14af7dd77c26f203cb58a8bc6300a4a5c` | 9704 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/deploy-instance/SKILL.md` | `deploy-instance` | `a8cba5eab37b24a63b4bd64df2b46c1f23e393c2eedec7a0b983e0a504ea6a40` | 13354 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/enumerate-changes/SKILL.md` | `enumerate-changes` | `841b80fd7c38495936e54056041bb25206fa5c3e33fb672a0e089771c025ceb3` | 10825 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/implement-milestone/SKILL.md` | `implement-milestone` | `641a273b7aa051e8971cb1873b2c7bf5d89f4acf935f8cf706dbaa2c3474661d` | 9761 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/investigate-issue/SKILL.md` | `investigate-issue` | `227f8a4f590e58db97e39170ec9eede597f5f369aee1074cbce6ec38409b0205` | 9693 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/plan-roadmap/SKILL.md` | `plan-roadmap` | `24de5037d629765a367d14b73605ca9329518da5227d4162c996bd85fc26b7cd` | 10730 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/preserve-mastodon-api-compat/SKILL.md` | `preserve-mastodon-api-compat` | `b5081eb525243ab3d393ab0122dde6f6d87b97baebef9d40bf35726a516f6c46` | 13403 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/protect-federation-trust/SKILL.md` | `protect-federation-trust` | `69ae488636fc4bd8d4ce587a1dbcc2a5c866dd01dde7924209f584fa7664f7db` | 13143 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/review-advisor-brief/SKILL.md` | `review-advisor-brief` | `9be0b1877c959f1687d59b687e8449ce6c3692572328861d95d374cf33c750bf` | 9940 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/scope-need/SKILL.md` | `scope-need` | `5264637e109a0b312354f0e07da8900137832eae87956aecc3b6c150a7411e46` | 11220 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `.codex/skills/validate-schema/SKILL.md` | `validate-schema` | `9a1399a0b863a32a31c4dc6bb2bfc3e55b4e62f4f02bd5ccef25ef87da688d25` | 14040 | preferred Codex-runtime seed; non-authoritative until imported | manifest, approval, exposure, capabilities, bundle metadata |
| `skills/high-risk-domain-planning/SKILL.md` | `high-risk-domain-planning` | `464a2a51aae6fa1e71b2bebc0395a8635f7cb28a6451ad6d76830d616b816068` | 2932 | repo-root shared seed; import-proposal candidate | manifest, approval, exposure, capabilities, bundle metadata |

## Duplicate digest groups

These files should de-duplicate by digest during any future import while preserving runtime/source provenance.

- `9b1b9fd9ebba18303bf4ddf5f4bb29de350ed58b07272e05b92569969983c548`
  - `.claude/skills/coordinate-framework-feedback/SKILL.md`
  - `.codex/skills/coordinate-framework-feedback/SKILL.md`
- `8912e838cd1731231d221202f68942d14af7dd77c26f203cb58a8bc6300a4a5c`
  - `.claude/skills/create-github-project/SKILL.md`
  - `.codex/skills/create-github-project/SKILL.md`
- `a8cba5eab37b24a63b4bd64df2b46c1f23e393c2eedec7a0b983e0a504ea6a40`
  - `.claude/skills/deploy-instance/SKILL.md`
  - `.codex/skills/deploy-instance/SKILL.md`
- `841b80fd7c38495936e54056041bb25206fa5c3e33fb672a0e089771c025ceb3`
  - `.claude/skills/enumerate-changes/SKILL.md`
  - `.codex/skills/enumerate-changes/SKILL.md`
- `641a273b7aa051e8971cb1873b2c7bf5d89f4acf935f8cf706dbaa2c3474661d`
  - `.claude/skills/implement-milestone/SKILL.md`
  - `.codex/skills/implement-milestone/SKILL.md`
- `227f8a4f590e58db97e39170ec9eede597f5f369aee1074cbce6ec38409b0205`
  - `.claude/skills/investigate-issue/SKILL.md`
  - `.codex/skills/investigate-issue/SKILL.md`
- `24de5037d629765a367d14b73605ca9329518da5227d4162c996bd85fc26b7cd`
  - `.claude/skills/plan-roadmap/SKILL.md`
  - `.codex/skills/plan-roadmap/SKILL.md`
- `b5081eb525243ab3d393ab0122dde6f6d87b97baebef9d40bf35726a516f6c46`
  - `.claude/skills/preserve-mastodon-api-compat/SKILL.md`
  - `.codex/skills/preserve-mastodon-api-compat/SKILL.md`
- `69ae488636fc4bd8d4ce587a1dbcc2a5c866dd01dde7924209f584fa7664f7db`
  - `.claude/skills/protect-federation-trust/SKILL.md`
  - `.codex/skills/protect-federation-trust/SKILL.md`
- `9be0b1877c959f1687d59b687e8449ce6c3692572328861d95d374cf33c750bf`
  - `.claude/skills/review-advisor-brief/SKILL.md`
  - `.codex/skills/review-advisor-brief/SKILL.md`
- `5264637e109a0b312354f0e07da8900137832eae87956aecc3b6c150a7411e46`
  - `.claude/skills/scope-need/SKILL.md`
  - `.codex/skills/scope-need/SKILL.md`
- `9a1399a0b863a32a31c4dc6bb2bfc3e55b4e62f4f02bd5ccef25ef87da688d25`
  - `.claude/skills/validate-schema/SKILL.md`
  - `.codex/skills/validate-schema/SKILL.md`

## Publishability gaps

The existing files are workspace instructions, not publishable canonical skill revisions. Import/promotion must account for these gaps:

1. **No canonical manifest.** Front matter has `name` and `description`, but no stable skill ID, version, exposure policy, capabilities, file manifest, or bundle digest.
2. **No approval metadata.** Files do not identify approving principal, approval digest/signature, review state, or revocation status.
3. **Mixed runtime targeting.** `.claude` and `.codex` copies are duplicated for local agent runtimes; canonical revisions should de-duplicate by digest while preserving runtime compatibility metadata.
4. **No public/private exposure policy.** Some skills are steward-internal and should not become public catalog entries by default.
5. **No provenance chain.** The repo path and commit can be source provenance, but imports need canonical `SkillProposal` records and `SkillRevision` provenance refs.
6. **No bundle shape.** Runtime publication later needs files, digests, install hints, compatibility targets, and AGPL/source provenance.

## Model fields informed by inventory

The inventory supports these fields in the M3 foundation model:

- `Skill.slug`, `Skill.name`, `Skill.description`
- `SkillRevision.manifestDigest`, `bundleDigest`, `contentDigest`, and `files[]`
- `SkillRevision.capabilities` and `Skill.tags`
- `SkillProposal.sourceType`, `sourceURI`, `sourceDigest`
- `SkillProvenanceRef` for repo path / commit / digest provenance
- `defaultExposure` / assignment `exposure` for public, instance, and private boundaries
