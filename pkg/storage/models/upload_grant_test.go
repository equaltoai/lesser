package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUploadGrantUpdateKeys(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.Add(15 * time.Minute)
	grant := &UploadGrant{
		Owner: "alice", GrantID: "grant-1", GrantedAt: now, ExpiresAt: expiry,
	}
	require.NoError(t, grant.UpdateKeys())
	require.Equal(t, "USER#alice#UPLOAD", grant.PK)
	require.Equal(t, "GRANT#grant-1", grant.SK)
	require.Equal(t, expiry.Unix(), grant.ExpiresAtTTL)
}

func TestUploadGrantUpdateKeysRejectsUnboundedGrant(t *testing.T) {
	// A zero expiry must be rejected: an upload grant without a bounded expiry
	// would authorize PUTs and finalize attempts indefinitely (fail-closed,
	// mirroring the review-grant discipline).
	grant := &UploadGrant{Owner: "alice", GrantID: "grant-1", GrantedAt: time.Now().UTC()}
	require.Error(t, grant.UpdateKeys())
}

func TestUploadGrantUpdateKeysRequiresIdentity(t *testing.T) {
	now := time.Now().UTC()
	require.Error(t, (&UploadGrant{GrantID: "g", ExpiresAt: now.Add(time.Minute)}).UpdateKeys(), "owner required")
	require.Error(t, (&UploadGrant{Owner: "alice", ExpiresAt: now.Add(time.Minute)}).UpdateKeys(), "grantID required")
}

func TestUploadGrantStatusPredicates(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	minted := &UploadGrant{Status: UploadGrantStatusMinted, ExpiresAt: future}
	require.True(t, minted.IsMinted())
	require.False(t, minted.IsUsed())
	require.False(t, minted.IsFailedDigest())
	require.False(t, minted.Expired(now))
	require.Equal(t, UploadGrantStatusMinted, minted.StatusClassification(now))

	// A minted grant past its bounded expiry classifies as EXPIRED and fails
	// closed everywhere.
	expired := &UploadGrant{Status: UploadGrantStatusMinted, ExpiresAt: past}
	require.True(t, expired.Expired(now))
	require.Equal(t, "EXPIRED", expired.StatusClassification(now))

	used := &UploadGrant{Status: UploadGrantStatusUsed, ExpiresAt: future}
	require.True(t, used.IsUsed())
	require.False(t, used.Expired(now), "a consumed grant is never reported expired")
	require.Equal(t, UploadGrantStatusUsed, used.StatusClassification(now))

	failed := &UploadGrant{Status: UploadGrantStatusFailedDigest, ExpiresAt: future}
	require.True(t, failed.IsFailedDigest())
	require.Equal(t, UploadGrantStatusFailedDigest, failed.StatusClassification(now))
}

func TestUploadGrantBaseModelSurface(t *testing.T) {
	now := time.Now().UTC()
	grant := &UploadGrant{Owner: "alice", GrantID: "g", GrantedAt: now, ExpiresAt: now.Add(time.Minute)}
	require.NoError(t, grant.UpdateKeys())
	require.Equal(t, MainTableName, grant.TableName())
	require.Equal(t, grant.PK, grant.GetPK())
	require.Equal(t, grant.SK, grant.GetSK())
}
