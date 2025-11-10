package advanced

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
	skReputation = "REPUTATION"
)

// safeIntToInt32 safely converts int to int32, capping at math.MaxInt32
func safeIntToInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n)
}

// ReputationScorer manages user reputation scoring
type ReputationScorer struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger
	config    *ModerationConfig

	// Cache for active scores
	scoreCache sync.Map
	cacheTTL   time.Duration
}

// NewReputationScorer creates a new reputation scorer
func NewReputationScorer(db *dynamodb.Client, tableName string, logger *zap.Logger, config *ModerationConfig) *ReputationScorer {
	return &ReputationScorer{
		db:        db,
		tableName: tableName,
		logger:    logger,
		config:    config,
		cacheTTL:  15 * time.Minute,
	}
}

// GetReputationScore retrieves or calculates a user's reputation score
func (rs *ReputationScorer) GetReputationScore(ctx context.Context, actorID string) (*ReputationScore, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("rep:%s", actorID)
	if cached, ok := rs.scoreCache.Load(cacheKey); ok {
		if score, ok := cached.(*cachedScore); ok && time.Since(score.cachedAt) < rs.cacheTTL {
			return score.score, nil
		}
	}

	// Get from DynamoDB
	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(rs.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actorID)},
			"SK": &types.AttributeValueMemberS{Value: skReputation},
		},
	}

	result, err := rs.db.GetItem(ctx, getInput)
	if err != nil {
		return nil, fmt.Errorf("get reputation: %w", err)
	}

	var score *ReputationScore

	if result.Item == nil {
		// New user, create default score
		score = rs.createDefaultScore(actorID)
		if err := rs.saveScore(ctx, score); err != nil {
			rs.logger.Warn("failed to save default score", zap.Error(err))
		}
	} else {
		score, err = rs.parseReputationScore(result.Item)
		if err != nil {
			return nil, fmt.Errorf("parse reputation: %w", err)
		}
	}

	// Apply decay if needed
	if rs.config.ReputationDecayRate > 0 && time.Since(score.UpdatedAt) > 24*time.Hour {
		score = rs.applyDecay(score)
		// Save decayed score asynchronously
		go func() {
			if err := rs.saveScore(context.Background(), score); err != nil {
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

// UpdateReputation updates a user's reputation based on an event
func (rs *ReputationScorer) UpdateReputation(ctx context.Context, actorID string, event ReputationEvent) error {
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
	if math.Abs(oldScore-score.Score) > 5 {
		rs.logger.Info("significant reputation change",
			zap.String("actorID", actorID),
			zap.Float64("oldScore", oldScore),
			zap.Float64("newScore", score.Score),
			zap.String("event", event.EventType))
	}

	// Record event for history
	if err := rs.recordEvent(ctx, actorID, event, impact); err != nil {
		rs.logger.Warn("failed to record reputation event", zap.Error(err))
	}

	// Clear cache
	rs.scoreCache.Delete(fmt.Sprintf("rep:%s", actorID))

	return nil
}

// GetReputationHistory retrieves reputation event history
func (rs *ReputationScorer) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]ReputationHistoryItem, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(rs.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actorID)},
			":prefix": &types.AttributeValueMemberS{Value: "EVENT#"},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(safeIntToInt32(limit)),
	}

	result, err := rs.db.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}

	history := make([]ReputationHistoryItem, 0, len(result.Items))
	for _, item := range result.Items {
		histItem, err := rs.parseHistoryItem(item)
		if err != nil {
			continue
		}
		history = append(history, *histItem)
	}

	return history, nil
}

// GetActorsByReputation retrieves actors within a reputation range
func (rs *ReputationScorer) GetActorsByReputation(ctx context.Context, minScore, maxScore float64, limit int) ([]*ReputationScore, error) {
	// This would require a GSI on reputation score
	// For now, we'll use a scan with filter (not efficient for large datasets)
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(rs.tableName),
		FilterExpression: aws.String("SK = :sk AND Score BETWEEN :min AND :max"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sk":  &types.AttributeValueMemberS{Value: skReputation},
			":min": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", minScore)},
			":max": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", maxScore)},
		},
		Limit: aws.Int32(safeIntToInt32(limit)),
	}

	result, err := rs.db.Scan(ctx, scanInput)
	if err != nil {
		return nil, fmt.Errorf("scan actors: %w", err)
	}

	scores := make([]*ReputationScore, 0, len(result.Items))
	for _, item := range result.Items {
		score, err := rs.parseReputationScore(item)
		if err != nil {
			continue
		}
		scores = append(scores, score)
	}

	return scores, nil
}

// CalculateReputationImpact calculates the reputation impact of a moderation decision
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
	item := map[string]types.AttributeValue{
		"PK":                 &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", score.ActorID)},
		"SK":                 &types.AttributeValueMemberS{Value: skReputation},
		"Score":              &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", score.Score)},
		"Level":              &types.AttributeValueMemberS{Value: score.Level},
		"ViolationCount":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", score.ViolationCount)},
		"FalsePositiveCount": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", score.FalsePositiveCount)},
		"ContentCount":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", score.ContentCount)},
		"UpdatedAt":          &types.AttributeValueMemberS{Value: score.UpdatedAt.Format(time.RFC3339)},

		// GSI for querying by level
		"gsi1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("LEVEL#%s", score.Level)},
		"gsi1SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SCORE#%06.2f#%s", score.Score, score.ActorID)},
	}

	// Add last violation if exists
	if !score.LastViolation.IsZero() {
		item["LastViolation"] = &types.AttributeValueMemberS{Value: score.LastViolation.Format(time.RFC3339)}
	}

	// Add factors
	if len(score.Factors) > 0 {
		factorList := &types.AttributeValueMemberL{
			Value: make([]types.AttributeValue, len(score.Factors)),
		}
		for i, factor := range score.Factors {
			factorMap := map[string]types.AttributeValue{
				"Factor":      &types.AttributeValueMemberS{Value: factor.Factor},
				"Impact":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", factor.Impact)},
				"Description": &types.AttributeValueMemberS{Value: factor.Description},
			}
			factorList.Value[i] = &types.AttributeValueMemberM{Value: factorMap}
		}
		item["Factors"] = factorList
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(rs.tableName),
		Item:      item,
	}

	_, err := rs.db.PutItem(ctx, putInput)
	if err != nil {
		return fmt.Errorf("save reputation score: %w", err)
	}

	return nil
}

func (rs *ReputationScorer) recordEvent(ctx context.Context, actorID string, event ReputationEvent, impact float64) error {
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	item := map[string]types.AttributeValue{
		"PK":          &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actorID)},
		"SK":          &types.AttributeValueMemberS{Value: fmt.Sprintf("EVENT#%d", timestamp.UnixNano())},
		"EventType":   &types.AttributeValueMemberS{Value: event.EventType},
		"Severity":    &types.AttributeValueMemberS{Value: string(event.Severity)},
		"Description": &types.AttributeValueMemberS{Value: event.Description},
		"Impact":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", impact)},
		"Timestamp":   &types.AttributeValueMemberS{Value: timestamp.Format(time.RFC3339)},
		"TTL":         &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", timestamp.Add(90*24*time.Hour).Unix())},
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(rs.tableName),
		Item:      item,
	}

	_, err := rs.db.PutItem(ctx, putInput)
	return err
}

func (rs *ReputationScorer) parseReputationScore(item map[string]types.AttributeValue) (*ReputationScore, error) {
	score := &ReputationScore{
		Factors: []ReputationFactor{},
	}

	// Parse all fields
	rs.parseActorID(item, score)
	rs.parseNumericFields(item, score)
	rs.parseStringFields(item, score)
	rs.parseTimestamps(item, score)
	rs.parseFactors(item, score)

	return score, nil
}

// parseActorID extracts the ActorID from the PK field
func (rs *ReputationScorer) parseActorID(item map[string]types.AttributeValue, score *ReputationScore) {
	if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
		score.ActorID = pk.Value[6:] // Remove "ACTOR#" prefix
	}
}

// parseNumericFields parses all numeric fields from the item
func (rs *ReputationScorer) parseNumericFields(item map[string]types.AttributeValue, score *ReputationScore) {
	rs.parseFloatField(item, "Score", &score.Score)
	rs.parseIntField(item, "ViolationCount", &score.ViolationCount)
	rs.parseIntField(item, "FalsePositiveCount", &score.FalsePositiveCount)
	rs.parseIntField(item, "ContentCount", &score.ContentCount)
}

// parseFloatField parses a single float field
func (rs *ReputationScorer) parseFloatField(item map[string]types.AttributeValue, key string, target *float64) {
	if v, ok := item[key].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%f", target); err != nil {
			rs.logger.Warn(fmt.Sprintf("failed to parse %s", key), zap.String("value", v.Value), zap.Error(err))
		}
	}
}

// parseIntField parses a single integer field
func (rs *ReputationScorer) parseIntField(item map[string]types.AttributeValue, key string, target *int) {
	if v, ok := item[key].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", target); err != nil {
			rs.logger.Warn(fmt.Sprintf("failed to parse %s", key), zap.String("value", v.Value), zap.Error(err))
		}
	}
}

// parseStringFields parses all string fields from the item
func (rs *ReputationScorer) parseStringFields(item map[string]types.AttributeValue, score *ReputationScore) {
	if v, ok := item["Level"].(*types.AttributeValueMemberS); ok {
		score.Level = v.Value
	}
}

// parseTimestamps parses timestamp fields from the item
func (rs *ReputationScorer) parseTimestamps(item map[string]types.AttributeValue, score *ReputationScore) {
	rs.parseTimestamp(item, "UpdatedAt", &score.UpdatedAt)
	rs.parseTimestamp(item, "LastViolation", &score.LastViolation)
}

// parseTimestamp parses a single timestamp field
func (rs *ReputationScorer) parseTimestamp(item map[string]types.AttributeValue, key string, target *time.Time) {
	if v, ok := item[key].(*types.AttributeValueMemberS); ok {
		*target, _ = time.Parse(time.RFC3339, v.Value)
	}
}

// parseFactors parses the reputation factors from the item
func (rs *ReputationScorer) parseFactors(item map[string]types.AttributeValue, score *ReputationScore) {
	v, ok := item["Factors"].(*types.AttributeValueMemberL)
	if !ok {
		return
	}

	for _, factorItem := range v.Value {
		if factor := rs.parseSingleFactor(factorItem); factor != nil {
			score.Factors = append(score.Factors, *factor)
		}
	}
}

// parseSingleFactor parses a single reputation factor
func (rs *ReputationScorer) parseSingleFactor(factorItem types.AttributeValue) *ReputationFactor {
	factorMap, ok := factorItem.(*types.AttributeValueMemberM)
	if !ok {
		return nil
	}

	factor := &ReputationFactor{}

	// Parse factor fields
	rs.parseFactorString(factorMap.Value, "Factor", &factor.Factor)
	rs.parseFactorFloat(factorMap.Value, "Impact", &factor.Impact)
	rs.parseFactorString(factorMap.Value, "Description", &factor.Description)

	return factor
}

// parseFactorString parses a string field from a factor
func (rs *ReputationScorer) parseFactorString(factorMap map[string]types.AttributeValue, key string, target *string) {
	if v, ok := factorMap[key].(*types.AttributeValueMemberS); ok {
		*target = v.Value
	}
}

// parseFactorFloat parses a float field from a factor
func (rs *ReputationScorer) parseFactorFloat(factorMap map[string]types.AttributeValue, key string, target *float64) {
	if v, ok := factorMap[key].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%f", target); err != nil {
			rs.logger.Warn(fmt.Sprintf("failed to parse factor %s", key), zap.String("value", v.Value), zap.Error(err))
		}
	}
}

// ReputationHistoryItem represents a reputation event in history
type ReputationHistoryItem struct {
	Timestamp   time.Time
	EventType   string
	Severity    Severity
	Description string
	Impact      float64
}

func (rs *ReputationScorer) parseHistoryItem(item map[string]types.AttributeValue) (*ReputationHistoryItem, error) {
	histItem := &ReputationHistoryItem{}

	if v, ok := item["EventType"].(*types.AttributeValueMemberS); ok {
		histItem.EventType = v.Value
	}
	if v, ok := item["Severity"].(*types.AttributeValueMemberS); ok {
		histItem.Severity = Severity(v.Value)
	}
	if v, ok := item["Description"].(*types.AttributeValueMemberS); ok {
		histItem.Description = v.Value
	}
	if v, ok := item["Impact"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%f", &histItem.Impact); err != nil {
			rs.logger.Warn("failed to parse Impact", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := item["Timestamp"].(*types.AttributeValueMemberS); ok {
		histItem.Timestamp, _ = time.Parse(time.RFC3339, v.Value)
	}

	return histItem, nil
}

type cachedScore struct {
	score    *ReputationScore
	cachedAt time.Time
}
