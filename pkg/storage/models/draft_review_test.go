package models

import (
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
}
