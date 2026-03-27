package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDirectMessageMigrationState_BeforeCreateSetsKeysAndTimestamps(t *testing.T) {
	state := &DirectMessageMigrationState{
		WritesFrozen: true,
		Phase:        "BACKFILL",
		Reason:       "migration",
		Owner:        "lesser migrate-direct-message-state",
	}

	require.NoError(t, state.BeforeCreate())
	require.Equal(t, DirectMessageMigrationStatePK, state.PK)
	require.Equal(t, DirectMessageMigrationStateSK, state.SK)
	require.False(t, state.CreatedAt.IsZero())
	require.False(t, state.UpdatedAt.IsZero())
}

func TestDirectMessageMigrationState_BeforeUpdatePreservesCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	state := &DirectMessageMigrationState{
		CreatedAt: createdAt,
	}

	require.NoError(t, state.BeforeUpdate())
	require.Equal(t, DirectMessageMigrationStatePK, state.PK)
	require.Equal(t, DirectMessageMigrationStateSK, state.SK)
	require.Equal(t, createdAt, state.CreatedAt)
	require.False(t, state.UpdatedAt.IsZero())
	require.True(t, state.UpdatedAt.After(createdAt))
}

func TestDirectMessageMigrationState_HelperAccessors(t *testing.T) {
	state := &DirectMessageMigrationState{}

	require.Equal(t, MainTableName, state.TableName())
	require.NoError(t, state.UpdateKeys())
	require.Equal(t, DirectMessageMigrationStatePK, state.GetPK())
	require.Equal(t, DirectMessageMigrationStateSK, state.GetSK())
}

func TestDirectMessageMigrationState_BeforeCreatePreservesProvidedTimestamps(t *testing.T) {
	createdAt := time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	state := &DirectMessageMigrationState{
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	require.NoError(t, state.BeforeCreate())
	require.Equal(t, createdAt, state.CreatedAt)
	require.Equal(t, updatedAt, state.UpdatedAt)
}

func TestDirectMessageMigrationState_BeforeUpdateSetsCreatedAtWhenMissing(t *testing.T) {
	state := &DirectMessageMigrationState{}

	require.NoError(t, state.BeforeUpdate())
	require.False(t, state.CreatedAt.IsZero())
	require.False(t, state.UpdatedAt.IsZero())
	require.Equal(t, state.UpdatedAt, state.CreatedAt)
}
