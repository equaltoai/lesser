package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_InstanceParity_Basics(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := &queryResolver{resolver}

	info, err := q.Instance(context.Background())
	require.NoError(t, err)
	require.NotNil(t, info)

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

