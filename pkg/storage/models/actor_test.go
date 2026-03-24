package models

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestActor_TableName(t *testing.T) {
	actor := Actor{}
	assert.Equal(t, MainTableName, actor.TableName())
}

func TestActor_BeforeCreate_NormalizesMixedCaseUsername(t *testing.T) {
	actor := &Actor{
		Username:      " Alice ",
		FollowerCount: 12,
		Actor: &activitypub.Actor{
			Name: "Alice",
		},
	}

	err := actor.BeforeCreate()

	assert.NoError(t, err)
	assert.Equal(t, "alice", actor.Username)
	assert.Equal(t, "ACTOR#alice", actor.PK)
	assert.Equal(t, SKProfile, actor.SK)
	assert.Equal(t, "USERNAME_SEARCH#al", actor.GSI1PK)
	assert.Equal(t, "alice", actor.GSI1SK)
	assert.Equal(t, "NAME_SEARCH#al", actor.GSI2PK)
	assert.Equal(t, "alice#alice", actor.GSI2SK)
	assert.Equal(t, "ACTOR_RANK#10+", actor.GSI4PK)
	assert.Equal(t, "0000000012#alice", actor.GSI4SK)
	assert.False(t, actor.CreatedAt.IsZero())
	assert.False(t, actor.UpdatedAt.IsZero())
}

func TestActor_BeforeUpdate(t *testing.T) {
	actor := &Actor{
		Username: "testuser",
		Actor: &activitypub.Actor{
			PreferredUsername: "testuser",
			Name:              "Updated User",
		},
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}

	oldUpdatedAt := actor.UpdatedAt

	err := actor.BeforeUpdate()

	assert.NoError(t, err)
	assert.True(t, actor.UpdatedAt.After(oldUpdatedAt))

	// Check GSI2 keys are updated for new display name
	assert.Equal(t, "NAME_SEARCH#up", actor.GSI2PK)
	assert.Equal(t, "updated user#testuser", actor.GSI2SK)
}

func TestActor_UpdateKeys_NormalizesMixedCaseUsername(t *testing.T) {
	actor := &Actor{
		Username:      " Alice ",
		FollowerCount: 3,
		Actor: &activitypub.Actor{
			Name: "Alice",
		},
	}

	err := actor.UpdateKeys()

	assert.NoError(t, err)
	assert.Equal(t, "alice", actor.Username)
	assert.Equal(t, "ACTOR#alice", actor.PK)
	assert.Equal(t, SKProfile, actor.SK)
	assert.Equal(t, "USERNAME_SEARCH#al", actor.GSI1PK)
	assert.Equal(t, "alice", actor.GSI1SK)
	assert.Equal(t, "alice#alice", actor.GSI2SK)
	assert.Equal(t, "0000000003#alice", actor.GSI4SK)
}

func TestActor_setupGSIKeys(t *testing.T) {
	tests := []struct {
		name           string
		username       string
		displayName    string
		followerCount  int
		expectedGSI1PK string
		expectedGSI1SK string
		expectedGSI2PK string
		expectedGSI2SK string
		expectedGSI4PK string
		expectedGSI4SK string
	}{
		{
			name:           "basic user",
			username:       "alice",
			displayName:    "Alice Smith",
			followerCount:  5,
			expectedGSI1PK: "USERNAME_SEARCH#al",
			expectedGSI1SK: "alice",
			expectedGSI2PK: "NAME_SEARCH#al",
			expectedGSI2SK: "alice smith#alice",
			expectedGSI4PK: "ACTOR_RANK#0-9",
			expectedGSI4SK: "0000000005#alice",
		},
		{
			name:           "popular user",
			username:       "bob",
			displayName:    "Bob Jones",
			followerCount:  15000,
			expectedGSI1PK: "USERNAME_SEARCH#bo",
			expectedGSI1SK: "bob",
			expectedGSI2PK: "NAME_SEARCH#bo",
			expectedGSI2SK: "bob jones#bob",
			expectedGSI4PK: "ACTOR_RANK#10K+",
			expectedGSI4SK: "0000015000#bob",
		},
		{
			name:           "short username",
			username:       "x",
			displayName:    "",
			followerCount:  0,
			expectedGSI1PK: "",
			expectedGSI1SK: "",
			expectedGSI2PK: "",
			expectedGSI2SK: "",
			expectedGSI4PK: "ACTOR_RANK#0-9",
			expectedGSI4SK: "0000000000#x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &Actor{
				Username:      tt.username,
				FollowerCount: tt.followerCount,
			}

			if tt.displayName != "" {
				actor.Actor = &activitypub.Actor{
					Name: tt.displayName,
				}
			}

			actor.setupGSIKeys()

			assert.Equal(t, tt.expectedGSI1PK, actor.GSI1PK)
			assert.Equal(t, tt.expectedGSI1SK, actor.GSI1SK)
			assert.Equal(t, tt.expectedGSI2PK, actor.GSI2PK)
			assert.Equal(t, tt.expectedGSI2SK, actor.GSI2SK)
			assert.Equal(t, tt.expectedGSI4PK, actor.GSI4PK)
			assert.Equal(t, tt.expectedGSI4SK, actor.GSI4SK)
		})
	}
}

func TestGetFollowerCountBucket(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{0, "0-9"},
		{5, "0-9"},
		{9, "0-9"},
		{10, "10+"},
		{50, "10+"},
		{99, "10+"},
		{100, "100+"},
		{500, "100+"},
		{999, "100+"},
		{1000, "1K+"},
		{5000, "1K+"},
		{9999, "1K+"},
		{10000, "10K+"},
		{50000, "10K+"},
		{100000, "10K+"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("count_%d", tt.count), func(t *testing.T) {
			result := getFollowerCountBucket(tt.count)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatFollowerCountForGSI(t *testing.T) {
	tests := []struct {
		count    int
		username string
		expected string
	}{
		{0, "alice", "0000000000#alice"},
		{42, "bob", "0000000042#bob"},
		{1337, "charlie", "0000001337#charlie"},
		{999999, "popular", "0000999999#popular"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("count_%d_%s", tt.count, tt.username), func(t *testing.T) {
			result := formatFollowerCountForGSI(tt.count, tt.username)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestActor_GSIKeysWithEmptyDisplayName(t *testing.T) {
	actor := &Actor{
		Username: "testuser",
		Actor: &activitypub.Actor{
			PreferredUsername: "testuser",
			Name:              "", // Empty display name
		},
	}

	actor.setupGSIKeys()

	// GSI1 should be set for username
	assert.Equal(t, "USERNAME_SEARCH#te", actor.GSI1PK)
	assert.Equal(t, "testuser", actor.GSI1SK)

	// GSI2 should be empty for empty display name
	assert.Empty(t, actor.GSI2PK)
	assert.Empty(t, actor.GSI2SK)
}

func TestActor_GSIKeysWithNilActor(t *testing.T) {
	actor := &Actor{
		Username: "testuser",
		Actor:    nil, // Nil actor
	}

	actor.setupGSIKeys()

	// GSI1 should be set for username
	assert.Equal(t, "USERNAME_SEARCH#te", actor.GSI1PK)
	assert.Equal(t, "testuser", actor.GSI1SK)

	// GSI2 should be empty for nil actor
	assert.Empty(t, actor.GSI2PK)
	assert.Empty(t, actor.GSI2SK)
}

func TestActor_GSI5RecentActivity(t *testing.T) {
	actor := &Actor{
		Username: "testuser",
	}

	// Capture time before setup
	beforeSetup := time.Now()

	actor.setupGSIKeys()

	// Capture time after setup
	afterSetup := time.Now()

	// GSI5PK should be today's date
	expectedDatePrefix := "ACTIVE#" + time.Now().Format(common.DateFormat)
	assert.Equal(t, expectedDatePrefix, actor.GSI5PK)

	// GSI5SK should contain timestamp and username
	assert.Contains(t, actor.GSI5SK, "#testuser")

	// Extract timestamp from GSI5SK
	parts := strings.Split(actor.GSI5SK, "#")
	assert.Len(t, parts, 2)
	assert.Equal(t, "testuser", parts[1])

	// Verify timestamp is reasonable (between before and after)
	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	assert.NoError(t, err)
	assert.True(t, timestamp >= beforeSetup.Unix())
	assert.True(t, timestamp <= afterSetup.Unix())
}
