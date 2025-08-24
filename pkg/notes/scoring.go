// Package notes provides community note scoring algorithms and visibility calculations for content annotation.
package notes

import (
	"github.com/equaltoai/lesser/pkg/common"
	"math"
	"sort"
	"time"
)

// CalculateNoteScore computes visibility score based on multiple factors
func CalculateNoteScore(note *CommunityNote, votes []Vote) float64 {
	// Base score components
	authorScore := normalizeReputation(note.AuthorRep)

	// Vote scoring with reputation weighting
	var helpfulWeight, notHelpfulWeight float64
	for _, vote := range votes {
		switch vote.VoteType {
		case VoteHelpful:
			helpfulWeight += vote.Weight
		case VoteNotHelpful:
			notHelpfulWeight += vote.Weight
		}
	}

	// Wilson score interval for ranking
	totalVotes := helpfulWeight + notHelpfulWeight
	if totalVotes == 0 {
		// Initial score based on author reputation and AI analysis
		return calculateInitialScore(note, authorScore)
	}

	wilsonScore := calculateWilsonScore(helpfulWeight, totalVotes)

	// Factor in AI analysis
	aiScore := (note.Sentiment + note.Objectivity + note.SourceQuality) / 3.0

	// Time decay factor (notes lose relevance over time)
	ageHours := time.Since(note.CreatedAt).Hours()
	timeDecay := math.Exp(-ageHours / 168) // Half-life of 1 week

	// Combine factors
	finalScore := (wilsonScore*0.4 + // Community voting (40%)
		authorScore*0.2 + // Author reputation (20%)
		aiScore*0.2 + // AI quality analysis (20%)
		note.SourceQuality*0.2) * timeDecay // Source verification (20%)

	return finalScore
}

// calculateInitialScore computes score for notes without votes yet
func calculateInitialScore(note *CommunityNote, authorScore float64) float64 {
	// Weight author reputation more heavily for initial score
	aiScore := (note.Sentiment + note.Objectivity + note.SourceQuality) / 3.0

	return authorScore*0.5 + // Author reputation (50%)
		aiScore*0.3 + // AI analysis (30%)
		note.SourceQuality*0.2 // Source quality (20%)
}

// calculateWilsonScore uses Wilson score confidence interval for ranking
// This handles the problem of ranking items with different numbers of votes
func calculateWilsonScore(positive, total float64) float64 {
	if total == 0 {
		return 0
	}

	z := 1.96 // 95% confidence interval
	phat := positive / total

	return (phat + z*z/(2*total) - z*math.Sqrt((phat*(1-phat)+z*z/(4*total))/total)) / (1 + z*z/total)
}

// normalizeReputation converts reputation score to 0-1 range
func normalizeReputation(reputation float64) float64 {
	// Use logarithmic scaling to handle wide reputation ranges
	// Cap at 10000 for normalization
	if reputation <= 0 {
		return 0
	}
	normalized := math.Log10(reputation+1) / math.Log10(10001)
	if normalized > 1 {
		return 1
	}
	return normalized
}

// CalculateVoteWeight determines the weight of a vote based on voter reputation
func CalculateVoteWeight(voterRep float64, voteType VoteType) float64 {
	// Base weight from reputation
	baseWeight := normalizeReputation(voterRep)

	// Minimum weight to prevent spam
	if baseWeight < 0.1 {
		baseWeight = 0.1
	}

	// Neutral votes have less weight
	if voteType == VoteNeutral {
		baseWeight *= 0.5
	}

	return baseWeight
}

// DetermineVisibilityStatus determines if a note should be visible based on score
func DetermineVisibilityStatus(score float64) VisibilityStatus {
	if score >= VisibilityThreshold {
		return VisibilityVisible
	} else if score < DisputeThreshold {
		return VisibilityDisputed
	}
	return VisibilityPending
}

// CalculateNoteLimit determines how many notes a user can create per day
func CalculateNoteLimit(reputation float64) int {
	if reputation < MinReputationToCreateNotes {
		return 0
	}

	// Linear scaling: 1 note at 100 rep, up to 10 notes at 1000+ rep
	limit := int(reputation / 100)
	if limit > MaxNoteLimit {
		return MaxNoteLimit
	}
	if limit < BaseNoteLimit {
		return BaseNoteLimit
	}
	return limit
}

// RankNotesByTrust adjusts note ordering based on viewer's trust relationships
func RankNotesByTrust(notes []CommunityNote, _ string, trustScores map[string]float64) []CommunityNote {
	// Create a copy to avoid modifying original
	rankedNotes := make([]CommunityNote, len(notes))
	copy(rankedNotes, notes)

	// Adjust scores based on trust
	for i := range rankedNotes {
		authorID := rankedNotes[i].AuthorID
		if trustScore, exists := trustScores[authorID]; exists {
			// Boost score for trusted authors (up to 20% boost)
			rankedNotes[i].Score *= (1 + trustScore*0.2)
		}
	}

	// Sort by adjusted score
	sortNotesByScore(rankedNotes)

	return rankedNotes
}

// sortNotesByScore sorts notes by score in descending order using Go's optimized sort
func sortNotesByScore(notes []CommunityNote) {
	// Use Go's optimized sort.Slice for O(n log n) performance
	// This is a stable sort that maintains relative order for equal scores
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Score > notes[j].Score // Descending order
	})
}

// CalculateStats generates statistics for a set of notes
func CalculateStats(notes []CommunityNote) map[string]any {
	if err := common.ValidateSliceNotEmpty("notes", notes); err != nil {
		return map[string]any{
			"total":         0,
			"visible":       0,
			"disputed":      0,
			"average_score": 0,
		}
	}

	var totalScore float64
	var visible, disputed int

	for _, note := range notes {
		totalScore += note.Score
		switch note.VisibilityStatus {
		case VisibilityVisible:
			visible++
		case VisibilityDisputed:
			disputed++
		}
	}

	return map[string]any{
		"total":         len(notes),
		"visible":       visible,
		"disputed":      disputed,
		"average_score": totalScore / float64(len(notes)),
	}
}
