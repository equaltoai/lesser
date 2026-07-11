package advanced

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydbErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestReputationScorer_GetReputationScore_CreatesDefaultAndCaches(t *testing.T) {
	db := new(mocks.MockDB)
	readQuery := new(mocks.MockQuery)
	writeQuery := new(mocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Twice()
	db.On("Model", mock.Anything).Return(readQuery).Once()
	db.On("Model", mock.Anything).Return(writeQuery).Once()

	readQuery.On("Where", "PK", "=", "ACTOR#actor-1").Return(readQuery).Once()
	readQuery.On("Where", "SK", "=", skReputation).Return(readQuery).Once()
	readQuery.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

	writeQuery.On("CreateOrUpdate").Return(nil).Once()

	cfg := DefaultModerationConfig()
	cfg.ReputationDecayRate = 0
	rs := NewReputationScorer(db, zap.NewNop(), cfg)

	score, err := rs.GetReputationScore(context.Background(), "actor-1")
	require.NoError(t, err)
	require.NotNil(t, score)
	assert.Equal(t, "actor-1", score.ActorID)
	assert.Equal(t, reputationLevelNormal, score.Level)

	// Second call should use cache (no extra First/CreateOrUpdate).
	score2, err := rs.GetReputationScore(context.Background(), "actor-1")
	require.NoError(t, err)
	require.NotNil(t, score2)
	assert.Equal(t, score.Score, score2.Score)

	db.AssertExpectations(t)
	readQuery.AssertExpectations(t)
	writeQuery.AssertExpectations(t)
}

func TestReputationScorer_UpdateReputation_UpdatesCountsAndRecordsEvent(t *testing.T) {
	db := new(mocks.MockDB)
	readQuery := new(mocks.MockQuery)
	writeQuery := new(mocks.MockQuery)
	eventQuery := new(mocks.MockQuery)

	var savedScore *reputationScoreRecord
	var savedEvent *reputationEventRecord

	db.On("WithContext", mock.Anything).Return(db).Times(3)
	db.On("Model", mock.Anything).Return(readQuery).Once()
	db.On("Model", mock.Anything).Return(writeQuery).Once().Run(func(args mock.Arguments) {
		savedScore = args.Get(0).(*reputationScoreRecord)
	})
	db.On("Model", mock.Anything).Return(eventQuery).Once().Run(func(args mock.Arguments) {
		savedEvent = args.Get(0).(*reputationEventRecord)
	})

	readQuery.On("Where", "PK", "=", "ACTOR#actor-1").Return(readQuery).Once()
	readQuery.On("Where", "SK", "=", skReputation).Return(readQuery).Once()
	readQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*reputationScoreRecord)
		*dest = reputationScoreRecord{
			PK:        "ACTOR#actor-1",
			SK:        skReputation,
			Score:     50.0,
			Level:     reputationLevelNormal,
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		}
	}).Return(nil).Once()

	writeQuery.On("CreateOrUpdate").Return(nil).Once()

	// Event recording failures should not make UpdateReputation fail.
	eventQuery.On("Create").Return(assert.AnError).Once()

	cfg := DefaultModerationConfig()
	cfg.TrustedActorThreshold = 80
	cfg.BadActorThreshold = 20
	rs := NewReputationScorer(db, zap.NewNop(), cfg)

	eventTime := time.Now().UTC()
	event := ReputationEvent{
		EventType:   eventTypeViolation,
		Severity:    SeverityHigh,
		Description: "bad",
		Timestamp:   eventTime,
	}

	require.NoError(t, rs.UpdateReputation(context.Background(), "actor-1", event))

	require.NotNil(t, savedScore)
	assert.Equal(t, "ACTOR#actor-1", savedScore.PK)
	assert.Equal(t, skReputation, savedScore.SK)
	assert.Equal(t, 40.0, savedScore.Score)
	assert.Equal(t, reputationLevelNormal, savedScore.Level)
	assert.Equal(t, 1, savedScore.ViolationCount)
	assert.Equal(t, eventTime, savedScore.LastViolation)
	assert.NotZero(t, savedScore.UpdatedAt)
	assert.NotEmpty(t, savedScore.GSI1PK)
	assert.Contains(t, savedScore.GSI1SK, "SCORE#040.00#actor-1")

	require.NotNil(t, savedEvent)
	assert.Equal(t, "ACTOR#actor-1", savedEvent.PK)
	assert.Contains(t, savedEvent.SK, skEventPrefix)
	assert.Equal(t, eventTypeViolation, savedEvent.EventType)
	assert.Equal(t, SeverityHigh, savedEvent.Severity)
	assert.Equal(t, "bad", savedEvent.Description)
	assert.Equal(t, eventTime, savedEvent.Timestamp)
	assert.NotZero(t, savedEvent.TTL)

	db.AssertExpectations(t)
	readQuery.AssertExpectations(t)
	writeQuery.AssertExpectations(t)
	eventQuery.AssertExpectations(t)
}

func TestReputationScorer_GetReputationHistory_ParsesItems(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once()

	query.On("Where", "PK", "=", "ACTOR#actor-1").Return(query).Once()
	query.On("Where", "SK", "BEGINS_WITH", skEventPrefix).Return(query).Once()
	query.On("OrderBy", "SK", "DESC").Return(query).Once()
	query.On("Limit", 10).Return(query).Once()
	query.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]reputationEventRecord)
		*dest = []reputationEventRecord{
			{
				EventType:   eventTypeViolation,
				Severity:    SeverityHigh,
				Description: "bad",
				Impact:      -5.0,
				Timestamp:   time.Now().UTC(),
			},
		}
	}).Return(nil).Once()

	rs := NewReputationScorer(db, zap.NewNop(), DefaultModerationConfig())
	history, err := rs.GetReputationHistory(context.Background(), "actor-1", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, eventTypeViolation, history[0].EventType)
	assert.Equal(t, SeverityHigh, history[0].Severity)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestReputationScorer_GetActorsByReputation_ParsesItems(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once()

	query.On("Filter", "SK", "=", skReputation).Return(query).Once()
	query.On("Filter", "Score", "BETWEEN", []any{50.0, 80.0}).Return(query).Once()
	query.On("Limit", 10).Return(query).Once()
	query.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]reputationScoreRecord)
		*dest = []reputationScoreRecord{
			{
				PK:        "ACTOR#actor-1",
				SK:        skReputation,
				Score:     75.0,
				Level:     reputationLevelNormal,
				UpdatedAt: time.Now().UTC(),
			},
		}
	}).Return(nil).Once()

	rs := NewReputationScorer(db, zap.NewNop(), DefaultModerationConfig())
	scores, err := rs.GetActorsByReputation(context.Background(), 50, 80, 10)
	require.NoError(t, err)
	require.Len(t, scores, 1)
	assert.Equal(t, "actor-1", scores[0].ActorID)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestReputationScorer_CalculateReputationImpact_AppliesSeverityAndConfidence(t *testing.T) {
	rs := NewReputationScorer(nil, zap.NewNop(), DefaultModerationConfig())
	impact := rs.CalculateReputationImpact(&ModerationDecision{
		Decision:       ActionRemove,
		Confidence:     0.5,
		Reasons:        []DecisionReason{{Type: "x", Severity: SeverityHigh}},
		RequiresReview: true,
	})
	assert.Less(t, impact, 0.0)
}

func TestReputationScorer_clampScore_and_calculateLevel_coverBranches(t *testing.T) {
	cfg := DefaultModerationConfig()
	cfg.TrustedActorThreshold = 80
	cfg.BadActorThreshold = 20

	rs := NewReputationScorer(nil, zap.NewNop(), cfg)

	assert.Equal(t, float64(0), rs.clampScore(-1))
	assert.Equal(t, float64(100), rs.clampScore(101))
	assert.Equal(t, float64(42), rs.clampScore(42))

	assert.Equal(t, reputationLevelTrusted, rs.calculateLevel(90))
	assert.Equal(t, reputationLevelNormal, rs.calculateLevel(50))
	assert.Equal(t, reputationLevelSuspicious, rs.calculateLevel(30))
	assert.Equal(t, reputationLevelBadActor, rs.calculateLevel(10))
}

func TestReputationScorer_calculateEventImpact_CoversEventTypesAndModifiers(t *testing.T) {
	cfg := DefaultModerationConfig()
	cfg.TrustedActorThreshold = 80
	cfg.BadActorThreshold = 20

	rs := NewReputationScorer(nil, zap.NewNop(), cfg)

	violation := ReputationEvent{EventType: eventTypeViolation, Severity: SeverityHigh}

	trusted := &ReputationScore{Score: 90, ViolationCount: 2}
	neutral := &ReputationScore{Score: 50, ViolationCount: 2}
	bad := &ReputationScore{Score: 10, ViolationCount: 2}

	impactTrusted := rs.calculateEventImpact(violation, trusted)
	impactNeutral := rs.calculateEventImpact(violation, neutral)
	impactBad := rs.calculateEventImpact(violation, bad)

	assert.Greater(t, impactTrusted, impactNeutral)
	assert.Less(t, impactBad, impactNeutral)

	assert.Greater(t, rs.calculateEventImpact(ReputationEvent{EventType: eventTypeFalsePositive}, neutral), 0.0)
	assert.Greater(t, rs.calculateEventImpact(ReputationEvent{EventType: eventTypeGoodContent}, neutral), 0.0)
	assert.Less(t, rs.calculateEventImpact(ReputationEvent{EventType: eventTypeUserReport}, neutral), 0.0)
}

func TestReputationScorer_CalculateReputationImpact_CoversActionsAndSeverities(t *testing.T) {
	rs := NewReputationScorer(nil, zap.NewNop(), DefaultModerationConfig())

	for _, action := range []ModerationAction{
		ActionAllow,
		ActionFlag,
		ActionQuarantine,
		ActionRemove,
		ActionShadowBan,
		ActionReportToAuth,
	} {
		impact := rs.CalculateReputationImpact(&ModerationDecision{
			Decision:   action,
			Confidence: 0.5,
			Reasons: []DecisionReason{
				{Severity: SeverityCritical},
				{Severity: SeverityHigh},
				{Severity: SeverityMedium},
				{Severity: SeverityLow},
			},
		})
		if action == ActionAllow {
			assert.Equal(t, 0.0, impact)
			continue
		}
		assert.NotZero(t, impact)
	}
}

func TestReputationScorer_recordEvent_UsesNowWhenTimestampZero(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	var savedEvent *reputationEventRecord

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once().Run(func(args mock.Arguments) {
		savedEvent = args.Get(0).(*reputationEventRecord)
	})
	query.On("Create").Return(nil).Once()

	rs := NewReputationScorer(db, zap.NewNop(), DefaultModerationConfig())
	require.NoError(t, rs.recordEvent(context.Background(), "actor-1", ReputationEvent{
		EventType:   eventTypeGoodContent,
		Severity:    SeverityLow,
		Description: "ok",
	}, 1.0))

	require.NotNil(t, savedEvent)
	assert.NotZero(t, savedEvent.Timestamp)
	assert.Contains(t, savedEvent.SK, skEventPrefix)
	assert.Equal(t, eventTypeGoodContent, savedEvent.EventType)
	assert.Equal(t, SeverityLow, savedEvent.Severity)
	assert.Equal(t, "ok", savedEvent.Description)
	assert.Equal(t, 1.0, savedEvent.Impact)
	assert.NotZero(t, savedEvent.TTL)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestReputationScorer_GetReputationScore_PropagatesUnexpectedErrors(t *testing.T) {
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once()

	query.On("Where", "PK", "=", "ACTOR#actor-1").Return(query).Once()
	query.On("Where", "SK", "=", skReputation).Return(query).Once()
	query.On("First", mock.Anything).Return(assert.AnError).Once()

	rs := NewReputationScorer(db, zap.NewNop(), DefaultModerationConfig())
	_, err := rs.GetReputationScore(context.Background(), "actor-1")
	require.Error(t, err)
	assert.False(t, theorydbErrors.IsNotFound(err))

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}
