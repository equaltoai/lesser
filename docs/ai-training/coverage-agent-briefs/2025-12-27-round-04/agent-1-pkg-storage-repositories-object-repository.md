# Agent 1 Brief — `pkg/storage/repositories` ObjectRepository (model conversion + mention parsing + update history)

## Goal

Raise coverage for the large 0%-covered file:

- `pkg/storage/repositories/object_repository.go`

Focus on **pure logic** and **low-dependency conversions** first (no remote sync, no background jobs, no network).

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Prefer table-driven tests + `stretchr/testify`.
- If you need DB interactions, use DynamORM mocks (`github.com/pay-theory/dynamorm/pkg/mocks`).

## What to cover

### 1) `modelToActivityPubObject` (core conversion)

Target: `func (r *ObjectRepository) modelToActivityPubObject(...)`

Cover:

- Note conversion branch:
  - sets `ID`, `Type`, `Published`, `Updated`, `To`, `CC`, `Sensitive`
  - `InReplyTo` pointer → string assignment when present
  - parses `AttachmentJSON`, `TagJSON`, `ContextJSON` when non-empty (valid JSON)
  - handles invalid JSON strings without error (current code ignores unmarshal errors)
- Default conversion branch:
  - returns `map[string]any` with core fields
  - includes `to` and `cc` only when present (`To != nil`, `CC != nil`)

### 2) Mention parsing helpers (pure)

Targets:

- `parseMentionsFromTags`
- `parseMentionsFromAnySlice`
- `extractMentionFromMap`
- `parseMentionsFromTagSlice`
- `parseMentionsFromJSONString`

Cover:

- `[]any` path:
  - valid Mention map (`{"type":"Mention","href":"https://example.com/@alice"}`) is extracted
  - non-map items ignored
  - wrong `type` ignored
  - missing/empty `href` ignored
- `[]activitypub.Tag` path:
  - extracts `TagTypeMention` entries with non-empty `Href`
- JSON-string path:
  - valid JSON → extracted mentions
  - invalid JSON → returns empty slice

### 3) Update history conversions (minimal DB mocking)

Targets:

- `CreateUpdateHistory` (marshals `PreviousState` to JSON string; sets keys; calls `Create`)
- `GetUpdateHistory` (queries by PK; converts `PreviousState` JSON back to map; tolerates invalid JSON)

Approach:

- Use DynamORM mocks to validate query shape and inject results:
  - `db.WithContext(ctx).Model(&models.UpdateHistory{}).Where("PK","=", ...).OrderBy("SK","DESC").Limit(limit).All(&histories)`
- For `CreateUpdateHistory`, mock:
  - `db.WithContext(ctx).Model(updateHistory).Create()`

Assertions:

- `PreviousState` is serialized when provided and empty when nil
- invalid `PreviousState` JSON in stored model logs a warning but does not fail; returned `PreviousState` is nil/empty

## Deliverables

- New tests in `pkg/storage/repositories/`, suggested filename:
  - `object_repository_helpers_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

