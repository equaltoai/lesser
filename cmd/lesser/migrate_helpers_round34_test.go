package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestAppendConversationMigrationSample_LimitsAndDeduplicates(t *testing.T) {
	appendConversationMigrationSample(nil, "conv-ignored")

	samples := []string{}
	appendConversationMigrationSample(&samples, "   ")
	require.Empty(t, samples)

	appendConversationMigrationSample(&samples, "conv-1")
	appendConversationMigrationSample(&samples, "conv-1")
	appendConversationMigrationSample(&samples, "conv-2")
	appendConversationMigrationSample(&samples, "conv-3")
	appendConversationMigrationSample(&samples, "conv-4")
	appendConversationMigrationSample(&samples, "conv-5")
	appendConversationMigrationSample(&samples, "conv-6")

	require.Equal(t, []string{"conv-1", "conv-2", "conv-3", "conv-4", "conv-5"}, samples)
}

func TestSetMigrationStringAttribute_UpdatesAndAuditsGSIFields(t *testing.T) {
	item := map[string]types.AttributeValue{}
	audited := map[string]struct{}{}

	require.True(t, setMigrationStringAttribute(item, "GSI1PK", "USER#arch", audited))
	require.Equal(t, "USER#arch", strAttr(t, item["GSI1PK"]))
	require.Contains(t, audited, "GSI1PK")

	require.False(t, setMigrationStringAttribute(item, "GSI1PK", "USER#arch", audited))

	require.True(t, setMigrationStringAttribute(item, "statusID", "status-2", audited))
	require.Equal(t, "status-2", strAttr(t, item["statusID"]))
	require.NotContains(t, audited, "statusID")
}

func TestMigrationUsernameKeyHelpers(t *testing.T) {
	username, suffix, ok := splitActorKeyUsername("ACTOR#alice#followers")
	require.True(t, ok)
	require.Equal(t, "alice", username)
	require.Equal(t, "#followers", suffix)

	username, suffix, ok = splitActorKeyUsername("ACTOR#alice")
	require.True(t, ok)
	require.Equal(t, "alice", username)
	require.Empty(t, suffix)

	_, _, ok = splitActorKeyUsername("ACTOR#")
	require.False(t, ok)
	_, _, ok = splitActorKeyUsername("USER#alice")
	require.False(t, ok)

	value, ok := prefixedUsernameValue("following#alice", "following#")
	require.True(t, ok)
	require.Equal(t, "alice", value)

	_, ok = prefixedUsernameValue("followers#alice", "following#")
	require.False(t, ok)
}
