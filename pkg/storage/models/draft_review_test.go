package models

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDraftReviewGrantUpdateKeys(t *testing.T) {
	granted := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	grant := &DraftReviewGrant{OwnerID: "owner", DraftID: "draft", Reviewer: "reviewer", GrantedAt: granted}
	require.NoError(t, grant.UpdateKeys())
	require.Equal(t, "USER#owner#DRAFT#REVIEW", grant.PK)
	require.Equal(t, "GRANT#draft#REVIEWER#reviewer", grant.SK)
	require.Equal(t, "DRAFT#REVIEWER#reviewer", grant.GSI2PK)
	require.Contains(t, grant.GSI2SK, "OWNER#owner#DRAFT#draft")
	now := time.Now().UTC()
	grant.RevokedAt = &now
	require.NoError(t, grant.UpdateKeys())
	require.Empty(t, grant.GSI2PK)
	require.Empty(t, grant.GSI2SK)
	implicitTime := &DraftReviewGrant{OwnerID: "owner", DraftID: "draft-2", Reviewer: "reviewer"}
	require.NoError(t, implicitTime.UpdateKeys())
	require.False(t, implicitTime.GrantedAt.IsZero())
	require.Error(t, (&DraftReviewGrant{}).UpdateKeys())
	require.Equal(t, MainTableName, grant.TableName())
}

func TestDraftReviewVerdictUpdateKeys(t *testing.T) {
	recorded := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	verdict := &DraftReviewVerdict{OwnerID: "owner", DraftID: "draft", Reviewer: "reviewer", Verdict: "APPROVED", RecordedAt: recorded}
	require.NoError(t, verdict.UpdateKeys())
	require.Equal(t, "USER#owner#DRAFT#REVIEW", verdict.PK)
	require.True(t, strings.HasPrefix(verdict.SK, "VERDICT#draft#TIME#"))
	require.Contains(t, verdict.SK, "#REVIEWER#reviewer")
	implicitTime := &DraftReviewVerdict{OwnerID: "owner", DraftID: "draft-2", Reviewer: "reviewer", Verdict: "APPROVED"}
	require.NoError(t, implicitTime.UpdateKeys())
	require.False(t, implicitTime.RecordedAt.IsZero())
	require.Error(t, (&DraftReviewVerdict{}).UpdateKeys())
	require.Equal(t, MainTableName, verdict.TableName())
	field, ok := reflect.TypeOf(*verdict).FieldByName("ContentHash")
	require.True(t, ok)
	require.Equal(t, "attr:contentHash,omitempty", field.Tag.Get("theorydb"))
	require.Equal(t, "content_hash,omitempty", field.Tag.Get("json"))
}

func TestDraftReviewGrantExpiryIsFailClosed(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	revoked := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	active := &DraftReviewGrant{ExpiresAt: &future}
	require.True(t, active.IsActive(now))
	require.False(t, active.Expired(now))

	expired := &DraftReviewGrant{ExpiresAt: &past}
	require.False(t, expired.IsActive(now))
	require.True(t, expired.Expired(now))

	// A grant without an expiry cannot authorize anything (fail-closed for rows
	// created before the M2 expiry surface; re-sharing refreshes the grant).
	legacy := &DraftReviewGrant{}
	require.False(t, legacy.IsActive(now))
	require.True(t, legacy.Expired(now))

	// Revocation dominates expiry classification.
	revokedGrant := &DraftReviewGrant{ExpiresAt: &future, RevokedAt: &revoked}
	require.False(t, revokedGrant.IsActive(now))
	require.False(t, revokedGrant.Expired(now))

	require.False(t, (*DraftReviewGrant)(nil).IsActive(now))
}
