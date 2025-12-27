package notes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDetermineVisibilityStatus(t *testing.T) {
	require.Equal(t, VisibilityVisible, DetermineVisibilityStatus(VisibilityThreshold))
	require.Equal(t, VisibilityPending, DetermineVisibilityStatus(DisputeThreshold))
	require.Equal(t, VisibilityDisputed, DetermineVisibilityStatus(DisputeThreshold-0.0001))
}

func TestCalculateNoteLimit(t *testing.T) {
	require.Equal(t, 0, CalculateNoteLimit(0))
	require.Equal(t, 0, CalculateNoteLimit(MinReputationToCreateNotes-1))
	require.Equal(t, BaseNoteLimit, CalculateNoteLimit(MinReputationToCreateNotes))
	require.Equal(t, MaxNoteLimit, CalculateNoteLimit(5000))
}

func TestCalculateVoteWeight_MinimumAndNeutral(t *testing.T) {
	require.Equal(t, 0.1, CalculateVoteWeight(0, VoteHelpful))
	require.Equal(t, 0.05, CalculateVoteWeight(0, VoteNeutral))
}

func TestRankNotesByTrust_BoostsTrustedAuthors(t *testing.T) {
	notesIn := []CommunityNote{
		{ID: "a", AuthorID: "alice", Score: 1.0, VisibilityStatus: VisibilityVisible},
		{ID: "b", AuthorID: "bob", Score: 0.9, VisibilityStatus: VisibilityVisible},
	}
	trustScores := map[string]float64{
		"bob": 1.0, // boost by 20%
	}

	ranked := RankNotesByTrust(notesIn, "viewer", trustScores)
	require.Len(t, ranked, 2)
	require.Equal(t, "b", ranked[0].ID)
	require.Greater(t, ranked[0].Score, ranked[1].Score)
}

func TestCalculateStats_Empty(t *testing.T) {
	stats := CalculateStats(nil)
	require.Equal(t, 0, stats["total"])
	require.Equal(t, 0, stats["visible"])
	require.Equal(t, 0, stats["disputed"])
	require.Equal(t, 0, stats["average_score"])
}

func TestCalculateNoteScore_NoVotes_UsesInitialScore(t *testing.T) {
	note := &CommunityNote{
		AuthorRep:        1000,
		Sentiment:        0.8,
		Objectivity:      0.9,
		SourceQuality:    0.7,
		CreatedAt:        time.Now(),
		VisibilityStatus: VisibilityPending,
	}

	score := CalculateNoteScore(note, nil)
	require.Greater(t, score, 0.0)
}
