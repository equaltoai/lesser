# TableName() Remediation Playbook

**Purpose**  
Provide a quick reference for adding missing `TableName()` implementations and keeping the audit clean.

---

## Implementation Template

```go
// TableName returns the DynamoDB table backing <ModelName>.
func (<Receiver>) TableName() string {
	return MainTableName
}
```

**Guidelines**
- Place the method adjacent to other boilerplate methods (`BeforeCreate`, `UpdateKeys`, etc.).
- Keep the doc comment short; replace `<ModelName>` and `<Receiver>` with the concrete names.
- If the struct already embeds another type exposing `TableName()`, you can forward the call, but prefer to add the method explicitly for clarity.

---

## Workflow Checklist

1. Locate the model in `docs/MISSING_TABLENAME_COMPLETE_LIST.md` and move its row to `IN_PROGRESS`.
2. Add `TableName()` to the struct (and any related builders or wrapper types).
3. Run targeted tests covering the package (`go test ./pkg/storage/models/...` or the owning service).
4. Verify no new `ResourceNotFoundException` entries appear in local logs.
5. Update the tracker row to `DONE` and record yourself as `Assignee`.

---

## Verification & Guardrails

- **Manual Gate:** Re-run the TableName audit script (see repo tooling) before merging each phase to ensure the tracker contains only unfinished work.
- **CI Follow-up (TODO):** Add an automated lint/test that fails when any struct in `pkg/storage/models` lacks `TableName()`. Once the tracker is empty, wire the check into the pipeline and delete the manual list.

---

Keep this playbook close while executing the remediation phases so we maintain a consistent implementation style and avoid regressions.
