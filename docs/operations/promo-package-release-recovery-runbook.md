# Promo Package Release Recovery Runbook

Runbook for reconciling a promo package (`promo_package.md`) left in the
transient `releasing` reservation after a failed or crashed release. The
reservation is a deliberate fail-safe: while `status = releasing` is held,
release and composition are refused, so a blind retry can never create a second
public post. Reconciliation is a narrow, version-conditioned write that records
what the release actually did — never a re-release.

## When to use this runbook

A package is wedged when its row shows `status = releasing` and one of the
release steps below did not complete:

1. **Gate + reserve** — approvals and assets verified, `draft → releasing`
   version-conditioned write wins.
2. **Re-derive** — approvals re-verified after the reservation.
3. **Create** — the outbound Status/Note is created through the notes promo
   lane (`CreatePromoNote`).
4. **Stamp** — `releasing → released` write records the created Status ID
   (`releasedStatusId`).

A wedge is only reachable through step 1 (the reservation) or step 3→4 (post
created, stamp failed). Step 2 failures roll the reservation back
automatically. Step 3 failures roll the reservation back automatically unless
the rollback itself failed (rollback-failure wedge).

## Failure-mode table

| # | Wedge | Package state | Post live? | Status ID available? | Reconciliation |
|---|-------|---------------|------------|----------------------|----------------|
| A | Stamp write fails after `CreatePromoNote` (finalize error surfaced as `PromoPackageStampError`) | `status = releasing`, `releasedStatusId` empty | **YES** | Yes — surfaced in the error | **Stamp to released** (section below), using the surfaced ID |
| B | Process crashes after `CreatePromoNote` succeeds but before the stamp write | `status = releasing`, `releasedStatusId` empty | **YES** | No — must be **discovered** (query by author + content) | **Stamp to released**, but first locate the status ID |
| C | Process crashes between the reservation and `CreatePromoNote` | `status = releasing`, `releasedStatusId` empty | No | n/a | **Revert to draft** after confirming no post |
| D | `CreatePromoNote` fails AND the automatic rollback also fails | `status = releasing`, `releasedStatusId` empty | No | n/a | **Revert to draft** after confirming no post |

Wedge A is the stamp-failure window; wedges C and D are the crash/rollback
window with no outbound post. Wedge B is the crash variant of A — the post is
live but the ID was never surfaced, so it must be discovered before stamping.

## Locating the package row

The package record lives on the main table:

- `PK = USER#{ownerID}#PROMO#PACKAGE`
- `SK = PACKAGE#{packageID}`

The reconciliation writes are the same field-scoped, version-conditioned
writes the service uses (`MarkPromoPackageReleased` /
`RevertPromoPackageReleasing` in `pkg/storage/repositories/promo_package_repository.go`).
Every write is conditioned on the freshly read `modelVersion` (with the
migration-safe `attribute_not_exists(modelVersion)` disjunct) and writes
`modelVersion = <read> + 1`. Always **read the row first**, capture
`modelVersion`, and make the write in the same read-modify-write turn; a
concurrent write that bumps the version makes your write conflict instead of
silently clobbering it.

## Wedge A — stamp to released (status ID surfaced)

Safety precondition — verify **before** stamping:

1. The surfaced Status ID actually exists as a Status/Note record
   (`PK = SK = status#{statusID}`).
2. It belongs to this package's release: `authorID` is the package's
   `ownerID`, the post text matches the package's `postText`, and the attached
   media match the package's ordered asset set by `contentHash` (the canonical
   `sha256:` digests bound in the package).
3. The package row is still `status = releasing` with an empty
   `releasedStatusId` (the reservation was not already reconciled).

Reconciliation write (exact attributes):

```
status:            "released"
releasedStatusID:  <surfaced status ID>        # verified above
releasedAt:        <now> (UTC)
updatedAt:         <now> (UTC)
modelVersion:      <read> + 1                  # CAS: modelVersion = <read>
```

This records the post that already exists; it creates nothing and changes no
content.

## Wedge B — stamp to released (status ID must be discovered)

The crash happened after the post was created but before the stamp, so no ID
was surfaced. **Discover** the status before stamping:

1. List the owner's Status/Note records created at-or-after the reservation
   timestamp (the `releasing` write sets `updatedAt`; that timestamp is the
   floor).
2. The package's post — and only that post — matches the package's `postText`
   and the ordered attachment digests (`contentHash` per bound asset).
3. If exactly one status matches, treat its ID as the surfaced ID and follow
   wedge A. If none matches, the package is in wedge C/D (no post was created)
   — follow the revert path instead. If more than one matches, stop and
   escalate: a second matching post means a different failure mode.

## Wedges C and D — revert to draft (no post was created)

Safety precondition — verify **before** reverting:

1. Confirm **no** Status/Note exists for this package's release: no status ID
   was surfaced and the author+content discovery above finds no match.
2. The package row is still `status = releasing` with an empty
   `releasedStatusId`.

Reconciliation write (exact attributes):

```
status:            "draft"
updatedAt:         <now> (UTC)
modelVersion:      <read> + 1                  # CAS: modelVersion = <read>
```

This writes only the status and timestamp (never content, never a released
stamp), returning the package to the reviewable state so a corrected release
can run through the normal gate again. Only the reservation winner holds the
post-reservation version, so the CAS write cannot clobber a concurrent content
edit.

## Never

- Never re-run `releasePromoPackage` as the reconciliation — it would reserve
  again and, in wedges A/B, create a **second public post**.
- Never write the row unconditionally (without the `modelVersion` CAS) — the
  read-modify-write could overwrite a newer state.
- Never set `status = released` in wedges C/D: the package would read as
  released with no post behind it.
- Never set `status = draft` in wedges A/B: the live post would be orphaned
  from the package record and a later release could duplicate it.
