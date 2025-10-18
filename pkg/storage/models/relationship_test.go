package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRelationshipDomain(t *testing.T) {
	tests := []struct {
		name           string
		handle         string
		expectedDomain string
		expectedOK     bool
	}{
		{
			name:           "federated handle",
			handle:         "alice@remote.com",
			expectedDomain: "remote.com",
			expectedOK:     true,
		},
		{
			name:           "multiple @ symbols",
			handle:         "alice@bob@remote.com",
			expectedDomain: "remote.com",
			expectedOK:     true,
		},
		{
			name:           "uppercase domain",
			handle:         "alice@REMOTE.COM",
			expectedDomain: "remote.com",
			expectedOK:     true,
		},
		{
			name:           "local user without domain",
			handle:         "alice",
			expectedDomain: "",
			expectedOK:     false,
		},
		{
			name:           "localhost domain",
			handle:         "alice@localhost",
			expectedDomain: "",
			expectedOK:     false,
		},
		{
			name:           "empty domain",
			handle:         "alice@",
			expectedDomain: "",
			expectedOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain, ok := extractRelationshipDomain(tt.handle)
			assert.Equal(t, tt.expectedOK, ok)
			assert.Equal(t, tt.expectedDomain, domain)
		})
	}
}

func TestRelationshipRecord_UpdateKeys(t *testing.T) {
	t.Run("populates domain GSIs for federated relationships", func(t *testing.T) {
		// Remote user following local user
		record := &RelationshipRecord{
			PK:     "FOLLOW#alice@remote.com",
			SK:     "FOLLOWING#bob",
			GSI1PK: "FOLLOW#bob",
			GSI1SK: "FOLLOWER#alice@remote.com",
			State:  RelationshipAccepted,
		}

		err := record.UpdateKeys()
		require.NoError(t, err)

		// GSI2: Follower domain (remote user following local)
		assert.Equal(t, "FOLLOWER_DOMAIN#remote.com", record.GSI2PK)
		assert.Equal(t, "FOLLOWING#bob", record.GSI2SK)

		// GSI3: Not set because following user is local
		assert.Empty(t, record.GSI3PK)
		assert.Empty(t, record.GSI3SK)
	})

	t.Run("populates domain GSIs for local following remote", func(t *testing.T) {
		// Local user following remote user
		record := &RelationshipRecord{
			PK:     "FOLLOW#alice",
			SK:     "FOLLOWING#bob@remote.com",
			GSI1PK: "FOLLOW#bob@remote.com",
			GSI1SK: "FOLLOWER#alice",
			State:  RelationshipAccepted,
		}

		err := record.UpdateKeys()
		require.NoError(t, err)

		// GSI2: Not set because follower is local
		assert.Empty(t, record.GSI2PK)
		assert.Empty(t, record.GSI2SK)

		// GSI3: Following domain (local user following remote)
		assert.Equal(t, "FOLLOWING_DOMAIN#remote.com", record.GSI3PK)
		assert.Equal(t, "FOLLOWER#alice", record.GSI3SK)
	})

	t.Run("populates both GSIs for remote-to-remote relationship", func(t *testing.T) {
		// Remote user following another remote user (edge case)
		record := &RelationshipRecord{
			PK:     "FOLLOW#alice@domain1.com",
			SK:     "FOLLOWING#bob@domain2.com",
			GSI1PK: "FOLLOW#bob@domain2.com",
			GSI1SK: "FOLLOWER#alice@domain1.com",
			State:  RelationshipAccepted,
		}

		err := record.UpdateKeys()
		require.NoError(t, err)

		// GSI2: Follower from domain1.com
		assert.Equal(t, "FOLLOWER_DOMAIN#domain1.com", record.GSI2PK)
		assert.Equal(t, "FOLLOWING#bob@domain2.com", record.GSI2SK)

		// GSI3: Following to domain2.com
		assert.Equal(t, "FOLLOWING_DOMAIN#domain2.com", record.GSI3PK)
		assert.Equal(t, "FOLLOWER#alice@domain1.com", record.GSI3SK)
	})

	t.Run("skips GSIs for local-to-local relationship", func(t *testing.T) {
		// Local user following local user
		record := &RelationshipRecord{
			PK:     "FOLLOW#alice",
			SK:     "FOLLOWING#bob",
			GSI1PK: "FOLLOW#bob",
			GSI1SK: "FOLLOWER#alice",
			State:  RelationshipAccepted,
		}

		err := record.UpdateKeys()
		require.NoError(t, err)

		// Neither GSI should be set for local relationships
		assert.Empty(t, record.GSI2PK)
		assert.Empty(t, record.GSI2SK)
		assert.Empty(t, record.GSI3PK)
		assert.Empty(t, record.GSI3SK)
	})
}

func TestNewRelationshipRecord(t *testing.T) {
	t.Run("creates record with proper keys", func(t *testing.T) {
		record := NewRelationshipRecord("alice", "bob@remote.com", "activity123")

		assert.Equal(t, "FOLLOW#alice", record.PK)
		assert.Equal(t, "FOLLOWING#bob@remote.com", record.SK)
		assert.Equal(t, "FOLLOW#bob@remote.com", record.GSI1PK)
		assert.Equal(t, "FOLLOWER#alice", record.GSI1SK)
		assert.Equal(t, "activity123", record.ActivityID)
		assert.Equal(t, RelationshipPending, record.State)
	})
}
