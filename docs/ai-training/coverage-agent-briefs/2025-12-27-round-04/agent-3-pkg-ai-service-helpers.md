# Agent 3 Brief — `pkg/ai` AIService helper logic (no AWS calls)

## Goal

Bring `pkg/ai/service.go` off 0% by adding unit tests for **pure helper logic** (no AWS clients invoked).

Primary target:

- `pkg/ai/service.go`

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Do not call methods that perform AWS or HTTP work (e.g., `analyzeText`, `uploadImageToS3`, `analyzeImages`, etc).
- Prefer table-driven tests + `stretchr/testify`.

## What to cover

### 1) Composite scoring helpers

Targets:

- `calculateOverallRisk`
- `calculateConfidence`

Cover combinations:

- no sub-analyses → `OverallRisk == 0`, `Confidence == 0.5`
- only text analysis
- text + spam + image + ai detection (verify weighted average and non-zero)
- `TextAnalysis.ContainsPII` increases risk per current weighting

### 2) Moderation action selection

Target:

- `determineModerationAction`

Set `AIService.config` thresholds and verify:

- NSFW override path triggers `ActionRemove` when `IsNSFW` and confidence exceeds threshold
- toxicity triggers `ActionHide`
- spam triggers `ActionShadowBan`
- fallback by `analysis.OverallRisk` buckets:
  - `> 0.9` remove, `> 0.7` hide, `> 0.5` flag, `> 0.3` review, else none

### 3) Text heuristics

Targets:

- `calculateRepetition`
- `analyzeTopicConsistency`
- `isCommonWord`
- `countMeaningfulWords`

Cover:

- repetition: empty / whitespace text returns 0; repeated word increases score
- topic consistency: 1-sentence content returns 1.0 (short-circuit); multi-sentence content returns value in `[0.5, 1.0]`
- common-word filter correctness for a few representative words

### 4) ID and URL parsing helpers

Targets:

- `generateID` (format: `<prefix>-<uuid>`)
- `extractS3Key`

Cover:

- empty url returns empty key
- S3 style URL and CloudFront URL return the path portion
- fallback returns last path segment as filename

## Deliverables

- New tests in `pkg/ai/`, suggested filename:
  - `service_helpers_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

