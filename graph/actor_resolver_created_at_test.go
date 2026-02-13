package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

func TestActorResolver_CreatedAt_Fallbacks(t *testing.T) {
	resolver := &actorResolver{}
	ctx := context.Background()

	created := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	actorWithCreated := &activitypub.Actor{CreatedAt: &created}
	got, err := resolver.CreatedAt(ctx, actorWithCreated)
	require.NoError(t, err)
	require.Equal(t, model.Time(created), *got)

	published := time.Date(2026, 2, 13, 12, 1, 0, 0, time.UTC)
	actorWithPublished := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{Published: &published},
	}
	got, err = resolver.CreatedAt(ctx, actorWithPublished)
	require.NoError(t, err)
	require.Equal(t, model.Time(published), *got)

	updated := time.Date(2026, 2, 13, 12, 2, 0, 0, time.UTC)
	actorWithUpdated := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{Updated: &updated},
	}
	got, err = resolver.CreatedAt(ctx, actorWithUpdated)
	require.NoError(t, err)
	require.Equal(t, model.Time(updated), *got)

	before := time.Now().UTC()
	got, err = resolver.CreatedAt(ctx, &activitypub.Actor{})
	after := time.Now().UTC()
	require.NoError(t, err)
	require.NotNil(t, got)
	require.False(t, time.Time(*got).Before(before))
	require.False(t, time.Time(*got).After(after))
}

