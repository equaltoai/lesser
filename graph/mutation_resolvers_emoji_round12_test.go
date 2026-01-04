package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_Emoji_AdminCreateUpdateDelete(t *testing.T) {
	resolver, _, _, mockQuery, _ := newRound12GraphResolverWithMocks(t)

	// Avoid pulling in boosted-state checks inside convertStatusToObject.
	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	ctx := round12AuthContext("admin")
	mut := resolver.Mutation()

	for _, call := range mockQuery.ExpectedCalls {
		if call.Method == "Count" {
			call.Unset()
		}
	}

	mockQuery.On("Count").Return(int64(0), nil).Once()  // create -> exists=false
	mockQuery.On("Count").Return(int64(1), nil).Once()  // update -> exists=true
	mockQuery.On("Count").Return(int64(1), nil).Once()  // delete -> exists=true
	mockQuery.On("Count").Return(int64(0), nil).Maybe() // fallback default

	created, err := mut.CreateEmoji(ctx, model.CreateEmojiInput{
		Shortcode: "wave",
		Image:     "https://example.com/wave.png",
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotEmpty(t, created.Shortcode)

	category := "custom"
	visible := true
	updated, err := mut.UpdateEmoji(ctx, "existing", model.UpdateEmojiInput{
		Category:        &category,
		VisibleInPicker: &visible,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	okBool, err := mut.DeleteEmoji(ctx, "existing")
	require.NoError(t, err)
	require.True(t, okBool)
}
