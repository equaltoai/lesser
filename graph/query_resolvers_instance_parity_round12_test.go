package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_InstanceParity_Basics(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	resolver.Config.Domain = "example.com"
	resolver.Config.WebSocketEndpoint = "https://ws.example.com/stream"
	resolver.Config.GraphQLWebSocketEndpoint = "https://ws.example.com"
	resolver.Config.MaxUploadSize = 12_345_678
	resolver.Config.MaxStatusChars = 777
	resolver.Config.CMSLongFormPublishingEnabled = true
	resolver.Config.CMSDraftSystemEnabled = true
	resolver.Config.CMSRevisionHistoryEnabled = false
	resolver.Config.CMSScheduledPublishingEnabled = true
	resolver.Config.CMSSeriesEnabled = false
	resolver.Config.CMSCategoriesEnabled = true
	q := &queryResolver{resolver}

	info, err := q.Instance(context.Background())
	require.NoError(t, err)
	require.NotNil(t, info)
	require.NotNil(t, info.StreamingURL)
	require.Equal(t, "wss://ws.example.com/stream", *info.StreamingURL)
	require.Equal(t, "wss://ws.example.com", info.SubscriptionURL)
	require.Equal(t, 12_345_678, info.MaxUploadSizeBytes)
	require.Equal(t, 777, info.MaxStatusCharacters)
	require.NotNil(t, info.CmsFeatures)
	require.True(t, info.CmsFeatures.LongForm)
	require.True(t, info.CmsFeatures.Drafts)
	require.False(t, info.CmsFeatures.Revisions)
	require.True(t, info.CmsFeatures.Scheduling)
	require.False(t, info.CmsFeatures.Series)
	require.True(t, info.CmsFeatures.Categories)

	activity, err := q.InstanceActivity(context.Background(), ptrIntValue(2))
	require.NoError(t, err)
	require.Len(t, activity, 2)

	peers, err := q.InstancePeers(context.Background(), ptrIntValue(10))
	require.NoError(t, err)
	require.NotNil(t, peers)

	blocks, err := q.InstanceDomainBlocks(context.Background(), ptrIntValue(10))
	require.NoError(t, err)
	require.NotNil(t, blocks)

	_, err = q.TranslationLanguages(context.Background())
	require.Error(t, err)
}

func TestGraphQLWebSocketURLFallbackAndNormalization(t *testing.T) {
	require.Equal(t, "wss://ws.example.com", graphQLWebSocketURL("", "example.com", ""))
	require.Equal(t, "ws://ws.localhost/stream", graphQLWebSocketURL("", "localhost", "/stream"))
	require.Equal(t, "wss://ws.example.com", graphQLWebSocketURL("https://ws.example.com", "ignored", ""))
	require.Equal(t, "ws://ws.example.com", graphQLWebSocketURL("http://ws.example.com", "ignored", ""))
}
