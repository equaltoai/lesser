package advanced

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// Reputation level constants
const (
	reputationLevelTrusted    = "trusted"
	reputationLevelNormal     = "normal"
	reputationLevelSuspicious = "suspicious"
	reputationLevelBadActor   = "bad_actor"
)

// Event type constants
const (
	eventTypeViolation     = "violation"
	eventTypeFalsePositive = "false_positive"
	eventTypeGoodContent   = "good_content"
	eventTypeUserReport    = "user_report"
)

// DynamoDB key constants
const (
	pkActorPrefix = "ACTOR#"
	skReputation  = "REPUTATION"
	skEventPrefix = "EVENT#"
)

type reputationScoreRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK"`

	Score              float64            `theorydb:"attr:score"`
	Level              string             `theorydb:"attr:level"`
	ViolationCount     int                `theorydb:"attr:violationCount"`
	FalsePositiveCount int                `theorydb:"attr:falsePositiveCount"`
	ContentCount       int                `theorydb:"attr:contentCount"`
	LastViolation      time.Time          `theorydb:"attr:lastViolation"`
	Factors            []ReputationFactor `theorydb:"attr:factors"`
	UpdatedAt          time.Time          `theorydb:"attr:updatedAt"`
}

func (reputationScoreRecord) TableName() string {
	return appconfig.GetMainTableName()
}

type reputationEventRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	EventType   string    `theorydb:"attr:eventType"`
	Severity    Severity  `theorydb:"attr:severity"`
	Description string    `theorydb:"attr:description"`
	Impact      float64   `theorydb:"attr:impact"`
	Timestamp   time.Time `theorydb:"attr:timestamp"`
	TTL         int64     `theorydb:"ttl,attr:ttl"`
}

func (reputationEventRecord) TableName() string {
	return appconfig.GetMainTableName()
}

// ReputationScorer manages user reputation scoring.
//
// It is intentionally TableTheory-backed: Lesser does not use direct DynamoDB SDK calls.
type ReputationScorer struct {
	db     core.DB
	logger *zap.Logger
	config *ModerationConfig

	// Cache for active scores
	scoreCache sync.Map
	cacheTTL   time.Duration
}

// NewReputationScorer creates a new reputation scorer.
func NewReputationScorer(db core.DB, logger *zap.Logger, config *ModerationConfig) *ReputationScorer {
	if config == nil {
		config = DefaultModerationConfig()
	}
	return &ReputationScorer{
		db:       db,
		logger:   logger,
		config:   config,
		cacheTTL: 15 * time.Minute,
	}
}

// GetReputationScore retrieves or calculates a user's reputation score.
func (rs *ReputationScorer) GetReputationScore(ctx context.Context, actorID string) (*ReputationScore, error) {
	if rs == nil || rs.db == nil {
		return nil, fmt.Errorf("reputation scorer is not initialized")
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, fmt.Errorf("actorID is required")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("rep:%s", actorID)
	if cached, ok := rs.scoreCache.Load(cacheKey); ok {
		if score, ok := cached.(*cachedScore); ok && time.Since(score.cachedAt) < rs.cacheTTL {
			return score.score, nil
		}
	}

	var record reputationScoreRecord
	if err := rs.db.WithContext(ctx).
		Model(&reputationScoreRecord{}).
		Where("PK", "=", reputationScorePK(actorID)).
		Where("SK", "=", skReputation).
		First(&record); err != nil {
		if !theorydbErrors.IsNotFound(err) {
			return nil, fmt.Errorf("get reputation: %w", err)
		}

		score := rs.createDefaultScore(actorID)
		if saveErr := rs.saveScore(ctx, score); saveErr != nil && rs.logger != nil {
			rs.logger.Warn("failed to save default reputation score", zap.Error(saveErr))
		}

		rs.scoreCache.Store(cacheKey, &cachedScore{
			score:    score,
			cachedAt: time.Now(),
		})

		return score, nil
	}

	score := rs.scoreFromRecord(&record)

	// Apply decay if needed
	if rs.config.ReputationDecayRate > 0 && time.Since(score.UpdatedAt) > 24*time.Hour {
		score = rs.applyDecay(score)
		// Save decayed score asynchronously
		go func() {
			if err := rs.saveScore(context.Background(), score); err != nil && rs.logger != nil {
				rs.logger.Warn("failed to save decayed score", zap.Error(err))
			}
		}()
	}

	// Cache the score
	rs.scoreCache.Store(cacheKey, &cachedScore{
		score:    score,
		cachedAt: time.Now(),
	})

	return score, nil
}

// UpdateReputation updates a user's reputation based on an event.
func (rs *ReputationScorer) UpdateReputation(ctx context.Context, actorID string, event ReputationEvent) error {
	if rs == nil || rs.db == nil {
		return fmt.Errorf("reputation scorer is not initialized")
	}

	// Get current score
	score, err := rs.GetReputationScore(ctx, actorID)
	if err != nil {
		return fmt.Errorf("get current score: %w", err)
	}

	// Apply event impact
	oldScore := score.Score
	impact := rs.calculateEventImpact(event, score)
	score.Score = rs.clampScore(score.Score + impact)

	// Update counts
	switch event.EventType {
	case eventTypeViolation:
		score.ViolationCount++
		score.LastViolation = event.Timestamp
	case eventTypeFalsePositive:
		score.FalsePositiveCount++
	case eventTypeGoodContent:
		score.ContentCount++
	}

	// Update level
	score.Level = rs.calculateLevel(score.Score)

	// Add factor
	score.Factors = append(score.Factors, ReputationFactor{
		Factor:      event.EventType,
		Impact:      impact,
		Description: event.Description,
	})

	// Keep only recent factors
	if len(score.Factors) > 20 {
		score.Factors = score.Factors[len(score.Factors)-20:]
	}

	score.UpdatedAt = time.Now()

	// Save updated score
	if err := rs.saveScore(ctx, score); err != nil {
		return fmt.Errorf("save score: %w", err)
	}

	// Log significant changes
	if rs.logger != nil && math.Abs(oldScore-score.Score) > 5 {
		rs.logger.Info("significant reputation change",
			zap.String("actorID", actorID),
			zap.Float64("oldScore", oldScore),
			zap.Float64("newScore", score.Score),
			zap.String("event", event.EventType))
	}

	// Record event for history
	if err := rs.recordEvent(ctx, actorID, event, impact); err != nil && rs.logger != nil {
		rs.logger.Warn("failed to record reputation event", zap.Error(err))
	}

	// Clear cache
	rs.scoreCache.Delete(fmt.Sprintf("rep:%s", actorID))

	return nil
}

// GetReputationHistory retrieves reputation event history.
func (rs *ReputationScorer) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]ReputationHistoryItem, error) {
	if rs == nil || rs.db == nil {
		return nil, fmt.Errorf("reputation scorer is not initialized")
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, fmt.Errorf("actorID is required")
	}
	if limit <= 0 {
		limit = 50
	}

	var records []reputationEventRecord
	if err := rs.db.WithContext(ctx).
		Model(&reputationEventRecord{}).
		Where("PK", "=", reputationScorePK(actorID)).
		Where("SK", "BEGINS_WITH", skEventPrefix).
		OrderBy("SK", "DESC").
		Limit(limit).
		All(&records); err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}

	history := make([]ReputationHistoryItem, 0, len(records))
	for _, record := range records {
		history = append(history, ReputationHistoryItem{
			Timestamp:   record.Timestamp,
			EventType:   record.EventType,
			Severity:    record.Severity,
			Description: record.Description,
			Impact:      record.Impact,
		})
	}

	return history, nil
}

// GetActorsByReputation retrieves actors within a reputation range.
func (rs *ReputationScorer) GetActorsByReputation(ctx context.Context, minScore, maxScore float64, limit int) ([]*ReputationScore, error) {
	if rs == nil || rs.db == nil {
		return nil, fmt.Errorf("reputation scorer is not initialized")
	}
	if limit <= 0 {
		limit = 100
	}

	var records []reputationScoreRecord
	if err := rs.db.WithContext(ctx).
		Model(&reputationScoreRecord{}).
		Filter("SK", "=", skReputation).
		Filter("Score", "BETWEEN", []any{minScore, maxScore}).
		Limit(limit).
		Scan(&records); err != nil {
		return nil, fmt.Errorf("scan actors: %w", err)
	}

	scores := make([]*ReputationScore, 0, len(records))
	for i := range records {
		scores = append(scores, rs.scoreFromRecord(&records[i]))
	}

	return scores, nil
}

// CalculateReputationImpact calculates the reputation impact of a moderation decision.
func (rs *ReputationScorer) CalculateReputationImpact(decision *ModerationDecision) float64 {
	impact := 0.0

	switch decision.Decision {
	case ActionRemove:
		impact = -10.0
	case ActionQuarantine:
		impact = -5.0
	case ActionFlag:
		impact = -2.0
	case ActionShadowBan:
		impact = -15.0
	case ActionReportToAuth:
		impact = -20.0
	}

	// Adjust based on confidence
	impact *= decision.Confidence

	// Adjust based on severity
	for _, reason := range decision.Reasons {
		switch reason.Severity {
		case SeverityCritical:
			impact *= 2.0
		case SeverityHigh:
			impact *= 1.5
		case SeverityMedium:
			impact *= 1.0
		case SeverityLow:
			impact *= 0.5
		}
	}

	return impact
}

// Helper methods

func (rs *ReputationScorer) createDefaultScore(actorID string) *ReputationScore {
	return &ReputationScore{
		ActorID:            actorID,
		Score:              50.0, // Start at neutral
		Level:              reputationLevelNormal,
		ViolationCount:     0,
		FalsePositiveCount: 0,
		ContentCount:       0,
		Factors:            []ReputationFactor{},
		UpdatedAt:          time.Now(),
	}
}

func (rs *ReputationScorer) calculateEventImpact(event ReputationEvent, currentScore *ReputationScore) float64 {
	baseImpact := 0.0

	switch event.EventType {
	case eventTypeViolation:
		baseImpact = -5.0
		switch event.Severity {
		case SeverityCritical:
			baseImpact = -15.0
		case SeverityHigh:
			baseImpact = -10.0
		case SeverityMedium:
			baseImpact = -5.0
		case SeverityLow:
			baseImpact = -2.0
		}

	case eventTypeFalsePositive:
		// Restore some reputation for false positives
		baseImpact = 3.0

	case eventTypeGoodContent:
		// Small positive impact for good behavior
		baseImpact = 1.0

	case eventTypeUserReport:
		// Negative impact when reported by other users
		baseImpact = -3.0
	}

	// Apply diminishing returns for repeated violations
	if event.EventType == eventTypeViolation && currentScore.ViolationCount > 0 {
		multiplier := 1.0 + (float64(currentScore.ViolationCount) * 0.1)
		baseImpact *= multiplier
	}

	// Apply trust bonus for consistently good actors
	if currentScore.Score > rs.config.TrustedActorThreshold {
		baseImpact *= 0.8 // 20% reduction in negative impact
	}

	// Apply penalty for bad actors
	if currentScore.Score < rs.config.BadActorThreshold {
		baseImpact *= 1.5 // 50% increase in negative impact
	}

	return baseImpact
}

func (rs *ReputationScorer) clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func (rs *ReputationScorer) calculateLevel(score float64) string {
	switch {
	case score >= rs.config.TrustedActorThreshold:
		return reputationLevelTrusted
	case score >= 40:
		return reputationLevelNormal
	case score >= rs.config.BadActorThreshold:
		return reputationLevelSuspicious
	default:
		return reputationLevelBadActor
	}
}

func (rs *ReputationScorer) applyDecay(score *ReputationScore) *ReputationScore {
	// Apply decay towards neutral (50)
	daysSinceUpdate := time.Since(score.UpdatedAt).Hours() / 24
	decayFactor := math.Pow(1-rs.config.ReputationDecayRate, daysSinceUpdate)

	// Decay towards 50 (neutral)
	score.Score = 50 + (score.Score-50)*decayFactor
	score.Level = rs.calculateLevel(score.Score)
	score.UpdatedAt = time.Now()

	return score
}

func (rs *ReputationScorer) saveScore(ctx context.Context, score *ReputationScore) error {
	if rs == nil || rs.db == nil {
		return fmt.Errorf("reputation scorer is not initialized")
	}
	if score == nil {
		return fmt.Errorf("reputation score is nil")
	}

	record := &reputationScoreRecord{
		PK: reputationScorePK(score.ActorID),
		SK: skReputation,

		GSI1PK: fmt.Sprintf("LEVEL#%s", score.Level),
		GSI1SK: fmt.Sprintf("SCORE#%06.2f#%s", score.Score, score.ActorID),

		Score:              score.Score,
		Level:              score.Level,
		ViolationCount:     score.ViolationCount,
		FalsePositiveCount: score.FalsePositiveCount,
		ContentCount:       score.ContentCount,
		LastViolation:      score.LastViolation,
		Factors:            score.Factors,
		UpdatedAt:          score.UpdatedAt,
	}

	if err := rs.db.WithContext(ctx).Model(record).CreateOrUpdate(); err != nil {
		return fmt.Errorf("save reputation score: %w", err)
	}

	return nil
}

func (rs *ReputationScorer) recordEvent(ctx context.Context, actorID string, event ReputationEvent, impact float64) error {
	if rs == nil || rs.db == nil {
		return fmt.Errorf("reputation scorer is not initialized")
	}
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	record := &reputationEventRecord{
		PK: reputationScorePK(actorID),
		SK: fmt.Sprintf("%s%d", skEventPrefix, timestamp.UnixNano()),

		EventType:   event.EventType,
		Severity:    event.Severity,
		Description: event.Description,
		Impact:      impact,
		Timestamp:   timestamp,
		TTL:         timestamp.Add(90 * 24 * time.Hour).Unix(),
	}

	if err := rs.db.WithContext(ctx).Model(record).Create(); err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	return nil
}

func (rs *ReputationScorer) scoreFromRecord(record *reputationScoreRecord) *ReputationScore {
	if record == nil {
		return nil
	}

	score := &ReputationScore{
		ActorID:            strings.TrimPrefix(record.PK, pkActorPrefix),
		Score:              record.Score,
		Level:              record.Level,
		ViolationCount:     record.ViolationCount,
		FalsePositiveCount: record.FalsePositiveCount,
		ContentCount:       record.ContentCount,
		LastViolation:      record.LastViolation,
		Factors:            record.Factors,
		UpdatedAt:          record.UpdatedAt,
	}
	if score.Factors == nil {
		score.Factors = []ReputationFactor{}
	}
	return score
}

func reputationScorePK(actorID string) string {
	return fmt.Sprintf("%s%s", pkActorPrefix, actorID)
}

// ReputationHistoryItem represents a reputation event in history.
type ReputationHistoryItem struct {
	Timestamp   time.Time
	EventType   string
	Severity    Severity
	Description string
	Impact      float64
}

type cachedScore struct {
	score    *ReputationScore
	cachedAt time.Time
}
