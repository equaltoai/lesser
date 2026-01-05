package reputation

import (
	"context"
	"math/rand"
	"testing"
	"testing/quick"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Test infrastructure for calculator tests

// testLogger returns a no-op logger for tests
func testLogger() *zap.Logger {
	return zap.NewNop()
}

// testInstanceURL is the default instance URL for tests
const testInstanceURL = "https://test.example.com"

// newTestCalculator creates a calculator for testing with no storage dependency
// (Calculator doesn't actually use storage in its calculation methods)
func newTestCalculator() *Calculator {
	return NewCalculator(nil, testInstanceURL, testLogger())
}

// Helper functions for creating test data

func makeVouch(active bool, revoked bool, expiresAt time.Time, voucherRep int, confidence float64) Vouch {
	return Vouch{
		Active:            active,
		Revoked:           revoked,
		ExpiresAt:         expiresAt,
		VoucherReputation: voucherRep,
		Confidence:        confidence,
	}
}

func makeTrustRelationship(from, to string, score float64, updatedAt time.Time) TrustRelationship {
	return TrustRelationship{
		FromActor:  from,
		ToActor:    to,
		TrustScore: score,
		UpdatedAt:  updatedAt,
	}
}

func makeModerationEvent(eventType, outcome string, severity int) ModerationEvent {
	return ModerationEvent{
		Type:     eventType,
		Outcome:  outcome,
		Severity: severity,
	}
}


// TestNewCalculator verifies the calculator is created with correct fields
func TestNewCalculator(t *testing.T) {
	logger := testLogger()
	instanceURL := "https://my-instance.com"

	calc := NewCalculator(nil, instanceURL, logger)

	require.NotNil(t, calc)
	require.Equal(t, instanceURL, calc.instanceURL)
	require.Equal(t, logger, calc.logger)
	require.Nil(t, calc.store) // store can be nil for calculation-only use
}

// TestCalculate verifies the Calculate function returns a complete Reputation
func TestCalculate(t *testing.T) {
	calc := newTestCalculator()
	ctx := context.Background()
	now := time.Now()

	input := &CalculationInput{
		ActorID:        "https://example.com/users/alice",
		PostCount:      100,
		FollowerCount:  50,
		FollowingCount: 30,
		AccountCreated: now.AddDate(-1, 0, 0), // 1 year old account
		LastActive:     now.Add(-1 * time.Hour),
		VouchesReceived: []Vouch{
			makeVouch(true, false, now.Add(24*time.Hour), 600, 0.8),
		},
	}

	rep, err := calc.Calculate(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, rep)

	// Verify identity fields
	require.Equal(t, input.ActorID, rep.ActorID)
	require.Equal(t, testInstanceURL, rep.InstanceURL)
	require.Equal(t, "1.0", rep.Version)

	// Verify evidence fields
	require.Equal(t, input.PostCount, rep.TotalPosts)
	require.Equal(t, input.FollowerCount, rep.TotalFollowers)
	require.Equal(t, 1, rep.VouchCount)

	// Verify all score components are populated (non-negative)
	require.GreaterOrEqual(t, rep.TrustScore, 0)
	require.GreaterOrEqual(t, rep.ActivityScore, 0)
	require.GreaterOrEqual(t, rep.ModerationScore, 0)
	require.GreaterOrEqual(t, rep.CommunityScore, 0)

	// Verify total score is sum of components
	expectedTotal := rep.TrustScore + rep.ActivityScore + rep.ModerationScore + rep.CommunityScore
	require.Equal(t, expectedTotal, rep.TotalScore)

	// Verify timestamp is recent
	require.WithinDuration(t, now, rep.CalculatedAt, 5*time.Second)
}


// TestCalculateTrustScore tests the trust score calculation
func TestCalculateTrustScore(t *testing.T) {
	calc := newTestCalculator()
	now := time.Now()
	actorID := "https://example.com/users/alice"

	t.Run("empty trust relationships returns 0", func(t *testing.T) {
		input := &CalculationInput{
			ActorID:            actorID,
			TrustRelationships: nil,
		}
		score := calc.calculateTrustScore(input)
		require.Equal(t, 0, score)

		input.TrustRelationships = []TrustRelationship{}
		score = calc.calculateTrustScore(input)
		require.Equal(t, 0, score)
	})

	t.Run("with incoming trust relationships", func(t *testing.T) {
		input := &CalculationInput{
			ActorID: actorID,
			TrustRelationships: []TrustRelationship{
				makeTrustRelationship("other1", actorID, 0.8, now),
				makeTrustRelationship("other2", actorID, 0.6, now),
			},
		}
		score := calc.calculateTrustScore(input)
		require.Greater(t, score, 0)
		require.LessOrEqual(t, score, 250)
	})

	t.Run("outgoing trust relationships are ignored", func(t *testing.T) {
		input := &CalculationInput{
			ActorID: actorID,
			TrustRelationships: []TrustRelationship{
				makeTrustRelationship(actorID, "other1", 0.9, now), // outgoing - should be ignored
			},
		}
		score := calc.calculateTrustScore(input)
		require.Equal(t, 0, score)
	})

	t.Run("recency weighting - recent vs old relationships", func(t *testing.T) {
		recentInput := &CalculationInput{
			ActorID: actorID,
			TrustRelationships: []TrustRelationship{
				makeTrustRelationship("other", actorID, 0.8, now.Add(-10*24*time.Hour)),
			},
		}
		recentScore := calc.calculateTrustScore(recentInput)

		oldInput := &CalculationInput{
			ActorID: actorID,
			TrustRelationships: []TrustRelationship{
				makeTrustRelationship("other", actorID, 0.8, now.Add(-60*24*time.Hour)),
			},
		}
		oldScore := calc.calculateTrustScore(oldInput)

		require.Greater(t, recentScore, oldScore)
	})


	t.Run("diversity bonus calculation", func(t *testing.T) {
		singleInput := &CalculationInput{
			ActorID: actorID,
			TrustRelationships: []TrustRelationship{
				makeTrustRelationship("other1", actorID, 0.5, now),
			},
		}
		singleScore := calc.calculateTrustScore(singleInput)

		multiInput := &CalculationInput{
			ActorID: actorID,
			TrustRelationships: []TrustRelationship{
				makeTrustRelationship("other1", actorID, 0.5, now),
				makeTrustRelationship("other2", actorID, 0.5, now),
				makeTrustRelationship("other3", actorID, 0.5, now),
			},
		}
		multiScore := calc.calculateTrustScore(multiInput)

		require.Greater(t, multiScore, singleScore)
	})

	t.Run("score capping at 250", func(t *testing.T) {
		relationships := make([]TrustRelationship, 20)
		for i := 0; i < 20; i++ {
			relationships[i] = makeTrustRelationship("other"+string(rune('a'+i)), actorID, 1.0, now)
		}

		input := &CalculationInput{
			ActorID:            actorID,
			TrustRelationships: relationships,
		}
		score := calc.calculateTrustScore(input)
		require.LessOrEqual(t, score, 250)
	})
}


// TestCalculateActivityScore tests the activity score calculation
func TestCalculateActivityScore(t *testing.T) {
	calc := newTestCalculator()
	now := time.Now()

	t.Run("account age scoring up to 50 points", func(t *testing.T) {
		newInput := &CalculationInput{
			AccountCreated: now,
			LastActive:     now,
		}
		newScore := calc.calculateActivityScore(newInput)

		yearInput := &CalculationInput{
			AccountCreated: now.AddDate(-1, 0, 0),
			LastActive:     now,
		}
		yearScore := calc.calculateActivityScore(yearInput)

		require.Greater(t, yearScore, newScore)
	})

	t.Run("post frequency scoring - ideal 1-5 posts per day", func(t *testing.T) {
		accountCreated := now.AddDate(0, 0, -100)

		lowInput := &CalculationInput{
			AccountCreated: accountCreated,
			PostCount:      10,
			LastActive:     now,
		}
		lowScore := calc.calculateActivityScore(lowInput)

		idealInput := &CalculationInput{
			AccountCreated: accountCreated,
			PostCount:      300,
			LastActive:     now,
		}
		idealScore := calc.calculateActivityScore(idealInput)

		require.Greater(t, idealScore, lowScore)
	})

	t.Run("over-posting penalty", func(t *testing.T) {
		accountCreated := now.AddDate(0, 0, -100)

		idealInput := &CalculationInput{
			AccountCreated: accountCreated,
			PostCount:      500,
			LastActive:     now,
		}
		idealScore := calc.calculateActivityScore(idealInput)

		overInput := &CalculationInput{
			AccountCreated: accountCreated,
			PostCount:      1000,
			LastActive:     now,
		}
		overScore := calc.calculateActivityScore(overInput)

		require.LessOrEqual(t, overScore, idealScore)
	})


	t.Run("follower count scoring - logarithmic", func(t *testing.T) {
		accountCreated := now.AddDate(-1, 0, 0)

		fewInput := &CalculationInput{
			AccountCreated: accountCreated,
			FollowerCount:  10,
			LastActive:     now,
		}
		fewScore := calc.calculateActivityScore(fewInput)

		manyInput := &CalculationInput{
			AccountCreated: accountCreated,
			FollowerCount:  10000,
			LastActive:     now,
		}
		manyScore := calc.calculateActivityScore(manyInput)

		require.Greater(t, manyScore, fewScore)
	})

	t.Run("recency bonus based on last activity", func(t *testing.T) {
		accountCreated := now.AddDate(-1, 0, 0)

		recentInput := &CalculationInput{
			AccountCreated: accountCreated,
			LastActive:     now.Add(-3 * 24 * time.Hour),
		}
		recentScore := calc.calculateActivityScore(recentInput)

		oldInput := &CalculationInput{
			AccountCreated: accountCreated,
			LastActive:     now.Add(-60 * 24 * time.Hour),
		}
		oldScore := calc.calculateActivityScore(oldInput)

		inactiveInput := &CalculationInput{
			AccountCreated: accountCreated,
			LastActive:     now.Add(-120 * 24 * time.Hour),
		}
		inactiveScore := calc.calculateActivityScore(inactiveInput)

		require.Greater(t, recentScore, oldScore)
		require.Greater(t, oldScore, inactiveScore)
	})

	t.Run("score capping at 250", func(t *testing.T) {
		input := &CalculationInput{
			AccountCreated: now.AddDate(-5, 0, 0),
			PostCount:      1000,
			FollowerCount:  1000000,
			LastActive:     now,
		}
		score := calc.calculateActivityScore(input)
		require.LessOrEqual(t, score, 250)
	})
}


// TestCalculateModerationScore tests the moderation score calculation
func TestCalculateModerationScore(t *testing.T) {
	calc := newTestCalculator()
	now := time.Now()

	t.Run("no violations returns 250", func(t *testing.T) {
		input := &CalculationInput{
			AccountCreated:    now.AddDate(-1, 0, 0),
			ModerationHistory: nil,
		}
		score := calc.calculateModerationScore(input)
		require.Equal(t, 250, score)

		input.ModerationHistory = []ModerationEvent{}
		score = calc.calculateModerationScore(input)
		require.Equal(t, 250, score)
	})

	t.Run("upheld reports deduct based on severity", func(t *testing.T) {
		lowSeverityInput := &CalculationInput{
			AccountCreated: now.AddDate(-1, 0, 0),
			ModerationHistory: []ModerationEvent{
				makeModerationEvent("report", OutcomeUpheld, 1),
			},
		}
		lowScore := calc.calculateModerationScore(lowSeverityInput)

		highSeverityInput := &CalculationInput{
			AccountCreated: now.AddDate(-1, 0, 0),
			ModerationHistory: []ModerationEvent{
				makeModerationEvent("report", OutcomeUpheld, 5),
			},
		}
		highScore := calc.calculateModerationScore(highSeverityInput)

		require.Less(t, lowScore, 250)
		require.Less(t, highScore, 250)
		require.Less(t, highScore, lowScore)
	})

	t.Run("dismissed reports add bonus", func(t *testing.T) {
		noReportsInput := &CalculationInput{
			AccountCreated:    now.AddDate(-1, 0, 0),
			ModerationHistory: nil,
		}
		noReportsScore := calc.calculateModerationScore(noReportsInput)

		dismissedInput := &CalculationInput{
			AccountCreated: now.AddDate(-1, 0, 0),
			ModerationHistory: []ModerationEvent{
				makeModerationEvent("report", OutcomeDismissed, 1),
			},
		}
		dismissedScore := calc.calculateModerationScore(dismissedInput)

		require.GreaterOrEqual(t, dismissedScore, noReportsScore)
	})


	t.Run("suspensions deduct 100 points", func(t *testing.T) {
		input := &CalculationInput{
			AccountCreated: now.AddDate(-1, 0, 0),
			ModerationHistory: []ModerationEvent{
				makeModerationEvent("suspension", "", 0),
			},
		}
		score := calc.calculateModerationScore(input)
		require.Equal(t, 150, score) // 250 - 100
	})

	t.Run("new account penalty for violations", func(t *testing.T) {
		newAccountInput := &CalculationInput{
			AccountCreated: now.Add(-15 * 24 * time.Hour),
			ModerationHistory: []ModerationEvent{
				makeModerationEvent("report", OutcomeUpheld, 1),
			},
		}
		newScore := calc.calculateModerationScore(newAccountInput)

		oldAccountInput := &CalculationInput{
			AccountCreated: now.AddDate(-1, 0, 0),
			ModerationHistory: []ModerationEvent{
				makeModerationEvent("report", OutcomeUpheld, 1),
			},
		}
		oldScore := calc.calculateModerationScore(oldAccountInput)

		require.Less(t, newScore, oldScore)
	})

	t.Run("score bounds 0-250", func(t *testing.T) {
		events := make([]ModerationEvent, 10)
		for i := 0; i < 10; i++ {
			events[i] = makeModerationEvent("suspension", "", 0)
		}

		input := &CalculationInput{
			AccountCreated:    now.Add(-15 * 24 * time.Hour),
			ModerationHistory: events,
		}
		score := calc.calculateModerationScore(input)
		require.GreaterOrEqual(t, score, 0)
		require.LessOrEqual(t, score, 250)
	})
}


// TestCalculateCommunityScore tests the community score calculation
func TestCalculateCommunityScore(t *testing.T) {
	calc := newTestCalculator()
	now := time.Now()
	future := now.Add(30 * 24 * time.Hour)

	t.Run("vouches received scoring", func(t *testing.T) {
		noVouchInput := &CalculationInput{
			VouchesReceived: nil,
		}
		noVouchScore := calc.calculateCommunityScore(noVouchInput)

		vouchInput := &CalculationInput{
			VouchesReceived: []Vouch{
				makeVouch(true, false, future, 800, 0.9),
				makeVouch(true, false, future, 600, 0.7),
			},
		}
		vouchScore := calc.calculateCommunityScore(vouchInput)

		require.Greater(t, vouchScore, noVouchScore)
	})

	t.Run("inactive and revoked vouches are not counted", func(t *testing.T) {
		activeInput := &CalculationInput{
			VouchesReceived: []Vouch{
				makeVouch(true, false, future, 800, 0.9),
			},
		}
		activeScore := calc.calculateCommunityScore(activeInput)

		inactiveInput := &CalculationInput{
			VouchesReceived: []Vouch{
				makeVouch(false, false, future, 800, 0.9),
			},
		}
		inactiveScore := calc.calculateCommunityScore(inactiveInput)

		revokedInput := &CalculationInput{
			VouchesReceived: []Vouch{
				makeVouch(true, true, future, 800, 0.9),
			},
		}
		revokedScore := calc.calculateCommunityScore(revokedInput)

		expiredInput := &CalculationInput{
			VouchesReceived: []Vouch{
				makeVouch(true, false, now.Add(-24*time.Hour), 800, 0.9),
			},
		}
		expiredScore := calc.calculateCommunityScore(expiredInput)

		require.Greater(t, activeScore, inactiveScore)
		require.Greater(t, activeScore, revokedScore)
		require.Greater(t, activeScore, expiredScore)
	})


	t.Run("vouches given scoring", func(t *testing.T) {
		noGivenInput := &CalculationInput{
			VouchesGiven: nil,
		}
		noGivenScore := calc.calculateCommunityScore(noGivenInput)

		givenInput := &CalculationInput{
			VouchesGiven: []Vouch{
				makeVouch(true, false, future, 0, 0),
				makeVouch(true, false, future, 0, 0),
			},
		}
		givenScore := calc.calculateCommunityScore(givenInput)

		require.Greater(t, givenScore, noGivenScore)
	})

	t.Run("community notes scoring", func(t *testing.T) {
		noNotesInput := &CalculationInput{
			CommunityNotes: 0,
		}
		noNotesScore := calc.calculateCommunityScore(noNotesInput)

		notesInput := &CalculationInput{
			CommunityNotes: 5,
		}
		notesScore := calc.calculateCommunityScore(notesInput)

		require.Greater(t, notesScore, noNotesScore)
	})

	t.Run("helpful votes scoring", func(t *testing.T) {
		noVotesInput := &CalculationInput{
			HelpfulVotes: 0,
		}
		noVotesScore := calc.calculateCommunityScore(noVotesInput)

		votesInput := &CalculationInput{
			HelpfulVotes: 30,
		}
		votesScore := calc.calculateCommunityScore(votesInput)

		require.Greater(t, votesScore, noVotesScore)
	})

	t.Run("score capping at 250", func(t *testing.T) {
		vouches := make([]Vouch, 20)
		for i := 0; i < 20; i++ {
			vouches[i] = makeVouch(true, false, future, 1000, 1.0)
		}

		input := &CalculationInput{
			VouchesReceived: vouches,
			VouchesGiven:    vouches,
			CommunityNotes:  100,
			HelpfulVotes:    100,
		}
		score := calc.calculateCommunityScore(input)
		require.LessOrEqual(t, score, 250)
	})
}


// TestGetBoostFromVouches tests the vouch boost calculation
func TestGetBoostFromVouches(t *testing.T) {
	calc := newTestCalculator()
	now := time.Now()
	future := now.Add(30 * 24 * time.Hour)

	t.Run("no vouches returns 0", func(t *testing.T) {
		boost := calc.GetBoostFromVouches(nil)
		require.Equal(t, 0, boost)

		boost = calc.GetBoostFromVouches([]Vouch{})
		require.Equal(t, 0, boost)
	})

	t.Run("active vouches calculate boost", func(t *testing.T) {
		vouches := []Vouch{
			makeVouch(true, false, future, 500, 0.8), // 500 * 0.8 / 10 = 40
			makeVouch(true, false, future, 600, 0.5), // 600 * 0.5 / 10 = 30
		}
		boost := calc.GetBoostFromVouches(vouches)
		require.Equal(t, 70, boost) // 40 + 30
	})

	t.Run("inactive vouches are skipped", func(t *testing.T) {
		vouches := []Vouch{
			makeVouch(false, false, future, 500, 0.8),
		}
		boost := calc.GetBoostFromVouches(vouches)
		require.Equal(t, 0, boost)
	})

	t.Run("revoked vouches are skipped", func(t *testing.T) {
		vouches := []Vouch{
			makeVouch(true, true, future, 500, 0.8),
		}
		boost := calc.GetBoostFromVouches(vouches)
		require.Equal(t, 0, boost)
	})

	t.Run("expired vouches are skipped", func(t *testing.T) {
		vouches := []Vouch{
			makeVouch(true, false, now.Add(-24*time.Hour), 500, 0.8),
		}
		boost := calc.GetBoostFromVouches(vouches)
		require.Equal(t, 0, boost)
	})


	t.Run("boost capping at 200", func(t *testing.T) {
		vouches := make([]Vouch, 10)
		for i := 0; i < 10; i++ {
			vouches[i] = makeVouch(true, false, future, 1000, 1.0) // 1000 * 1.0 / 10 = 100 each
		}
		boost := calc.GetBoostFromVouches(vouches)
		require.Equal(t, 200, boost) // Capped at 200
	})

	t.Run("mixed active and inactive vouches", func(t *testing.T) {
		vouches := []Vouch{
			makeVouch(true, false, future, 500, 0.8),                 // active: 40
			makeVouch(false, false, future, 500, 0.8),                // inactive: 0
			makeVouch(true, true, future, 500, 0.8),                  // revoked: 0
			makeVouch(true, false, now.Add(-24*time.Hour), 500, 0.8), // expired: 0
			makeVouch(true, false, future, 600, 0.5),                 // active: 30
		}
		boost := calc.GetBoostFromVouches(vouches)
		require.Equal(t, 70, boost) // Only active vouches counted
	})
}


// generateRandomCalculationInput creates a random CalculationInput for property testing
func generateRandomCalculationInput(r *rand.Rand) *CalculationInput {
	now := time.Now()
	actorID := "https://example.com/users/test"

	accountAgeDays := r.Intn(365 * 5)
	accountCreated := now.AddDate(0, 0, -accountAgeDays)

	lastActiveDays := r.Intn(180)
	lastActive := now.AddDate(0, 0, -lastActiveDays)

	numTrust := r.Intn(21)
	trustRels := make([]TrustRelationship, numTrust)
	for i := 0; i < numTrust; i++ {
		trustRels[i] = TrustRelationship{
			FromActor:  "other" + string(rune('a'+i)),
			ToActor:    actorID,
			TrustScore: r.Float64(),
			UpdatedAt:  now.AddDate(0, 0, -r.Intn(120)),
		}
	}

	numEvents := r.Intn(11)
	events := make([]ModerationEvent, numEvents)
	for i := 0; i < numEvents; i++ {
		eventType := "report"
		if r.Intn(5) == 0 {
			eventType = "suspension"
		}
		outcome := OutcomePending
		switch r.Intn(3) {
		case 0:
			outcome = OutcomeUpheld
		case 1:
			outcome = OutcomeDismissed
		}
		events[i] = ModerationEvent{
			Type:     eventType,
			Outcome:  outcome,
			Severity: r.Intn(5) + 1,
		}
	}

	numVouches := r.Intn(16)
	vouches := make([]Vouch, numVouches)
	for i := 0; i < numVouches; i++ {
		vouches[i] = Vouch{
			Active:            r.Intn(2) == 1,
			Revoked:           r.Intn(5) == 0,
			ExpiresAt:         now.AddDate(0, 0, r.Intn(60)-30),
			VoucherReputation: r.Intn(1000),
			Confidence:        r.Float64(),
		}
	}

	return &CalculationInput{
		ActorID:            actorID,
		PostCount:          r.Intn(10000),
		FollowerCount:      r.Intn(100000),
		FollowingCount:     r.Intn(10000),
		AccountCreated:     accountCreated,
		LastActive:         lastActive,
		TrustRelationships: trustRels,
		ModerationHistory:  events,
		VouchesReceived:    vouches,
		VouchesGiven:       vouches,
		CommunityNotes:     r.Intn(50),
		HelpfulVotes:       r.Intn(100),
	}
}


// TestProperty_ScoreBoundsInvariant verifies that all individual scores are within [0, 250]
// **Property 1: Score Bounds Invariant**
// **Validates: Requirements 1.4, 1.5, 1.9**
func TestProperty_ScoreBoundsInvariant(t *testing.T) {
	calc := newTestCalculator()

	property := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		input := generateRandomCalculationInput(r)

		trustScore := calc.calculateTrustScore(input)
		activityScore := calc.calculateActivityScore(input)
		moderationScore := calc.calculateModerationScore(input)
		communityScore := calc.calculateCommunityScore(input)

		if trustScore < 0 || trustScore > 250 {
			t.Logf("TrustScore out of bounds: %d", trustScore)
			return false
		}
		if activityScore < 0 || activityScore > 250 {
			t.Logf("ActivityScore out of bounds: %d", activityScore)
			return false
		}
		if moderationScore < 0 || moderationScore > 250 {
			t.Logf("ModerationScore out of bounds: %d", moderationScore)
			return false
		}
		if communityScore < 0 || communityScore > 250 {
			t.Logf("CommunityScore out of bounds: %d", communityScore)
			return false
		}

		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property failed: %v", err)
	}
}


// TestProperty_TotalScoreComposition verifies that TotalScore equals sum of all component scores
// **Property 2: Total Score Composition**
// **Validates: Requirements 1.2**
func TestProperty_TotalScoreComposition(t *testing.T) {
	calc := newTestCalculator()
	ctx := context.Background()

	property := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		input := generateRandomCalculationInput(r)

		rep, err := calc.Calculate(ctx, input)
		if err != nil {
			t.Logf("Calculate error: %v", err)
			return false
		}

		expectedTotal := rep.TrustScore + rep.ActivityScore + rep.ModerationScore + rep.CommunityScore
		if rep.TotalScore != expectedTotal {
			t.Logf("TotalScore mismatch: got %d, expected %d", rep.TotalScore, expectedTotal)
			return false
		}

		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property failed: %v", err)
	}
}


// TestProperty_VouchBoostCap verifies that vouch boost never exceeds 200
// **Property 3: Vouch Boost Cap**
// **Validates: Requirements 1.10**
func TestProperty_VouchBoostCap(t *testing.T) {
	calc := newTestCalculator()
	now := time.Now()

	property := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		numVouches := r.Intn(51)
		vouches := make([]Vouch, numVouches)
		for i := 0; i < numVouches; i++ {
			active := r.Intn(2) == 1
			revoked := r.Intn(5) == 0
			expiresAt := now.AddDate(0, 0, r.Intn(60)-30)
			voucherRep := r.Intn(2000)
			confidence := r.Float64()

			vouches[i] = Vouch{
				Active:            active,
				Revoked:           revoked,
				ExpiresAt:         expiresAt,
				VoucherReputation: voucherRep,
				Confidence:        confidence,
			}
		}

		boost := calc.GetBoostFromVouches(vouches)

		if boost > 200 {
			t.Logf("Boost exceeded cap: %d (numVouches=%d)", boost, numVouches)
			return false
		}

		if boost < 0 {
			t.Logf("Boost is negative: %d", boost)
			return false
		}

		return true
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property failed: %v", err)
	}
}
