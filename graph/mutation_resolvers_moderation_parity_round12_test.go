package graph

import (
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_ModerationParity_SubmitModerationReview(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	ctx := round12AuthContext("admin")

	notes := "looks good"
	out, err := resolver.Mutation().SubmitModerationReview(ctx, model.ModerationReviewInput{
		EventID:     "evt-1",
		Action:     "hide",
		Severity:   2,
		Confidence: 0.75,
		Notes:      &notes,
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotEmpty(t, out.ReviewID)
	require.Equal(t, "evt-1", out.EventID)

	_, err = resolver.Mutation().SubmitModerationReview(ctx, model.ModerationReviewInput{
		EventID:     "evt-2",
		Action:     "hide",
		Severity:   9,
		Confidence: 0.75,
	})
	require.Error(t, err)
}

