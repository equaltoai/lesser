package advanced

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// PatternMatcher handles pattern-based content matching
type PatternMatcher struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger

	// In-memory cache of active patterns
	patterns    sync.Map
	regexCache  sync.Map
	lastUpdate  time.Time
	updateMutex sync.RWMutex
}

// NewPatternMatcher creates a new pattern matcher
func NewPatternMatcher(db *dynamodb.Client, tableName string, logger *zap.Logger) *PatternMatcher {
	pm := &PatternMatcher{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}

	// Load patterns on initialization
	ctx := context.Background()
	if err := pm.loadPatterns(ctx); err != nil {
		logger.Warn("failed to load patterns on init", zap.Error(err))
	}

	// Start periodic refresh
	go pm.refreshPatternsPeriodically()

	return pm
}

// CreatePattern creates a new moderation pattern
func (pm *PatternMatcher) CreatePattern(ctx context.Context, pattern *ModerationPattern) error {
	// Validate pattern
	if err := pm.validatePattern(pattern); err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	// Generate ID if not provided
	if pattern.ID == "" {
		pattern.ID = generatePatternID(pattern.Name)
	}

	pattern.CreatedAt = time.Now()
	pattern.UpdatedAt = time.Now()
	pattern.HitCount = 0

	// Store in DynamoDB
	item := map[string]types.AttributeValue{
		"PK":          &types.AttributeValueMemberS{Value: fmt.Sprintf("PATTERN#%s", pattern.ID)},
		"SK":          &types.AttributeValueMemberS{Value: "METADATA"},
		"ID":          &types.AttributeValueMemberS{Value: pattern.ID},
		"Name":        &types.AttributeValueMemberS{Value: pattern.Name},
		"Description": &types.AttributeValueMemberS{Value: pattern.Description},
		"Pattern":     &types.AttributeValueMemberS{Value: pattern.Pattern},
		"PatternType": &types.AttributeValueMemberS{Value: pattern.PatternType},
		"Severity":    &types.AttributeValueMemberS{Value: string(pattern.Severity)},
		"Action":      &types.AttributeValueMemberS{Value: string(pattern.Action)},
		"CreatedBy":   &types.AttributeValueMemberS{Value: pattern.CreatedBy},
		"CreatedAt":   &types.AttributeValueMemberS{Value: pattern.CreatedAt.Format(time.RFC3339)},
		"UpdatedAt":   &types.AttributeValueMemberS{Value: pattern.UpdatedAt.Format(time.RFC3339)},
		"Active":      &types.AttributeValueMemberBOOL{Value: pattern.Active},
		"HitCount":    &types.AttributeValueMemberN{Value: "0"},

		// GSI for filtering
		"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERITY#%s", pattern.Severity)},
		"GSI1SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PATTERN#%s", pattern.ID)},
	}

	// Add categories
	if len(pattern.Categories) > 0 {
		categoryList := &types.AttributeValueMemberL{
			Value: make([]types.AttributeValue, len(pattern.Categories)),
		}
		for i, cat := range pattern.Categories {
			categoryList.Value[i] = &types.AttributeValueMemberS{Value: cat}
		}
		item["Categories"] = categoryList
	}

	putInput := &dynamodb.PutItemInput{
		TableName:           aws.String(pm.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	_, err := pm.db.PutItem(ctx, putInput)
	if err != nil {
		return fmt.Errorf("create pattern: %w", err)
	}

	// Update in-memory cache
	pm.patterns.Store(pattern.ID, pattern)

	// Pre-compile regex if applicable
	if pattern.PatternType == "regex" {
		if regex, err := regexp.Compile(pattern.Pattern); err == nil {
			pm.regexCache.Store(pattern.ID, regex)
		}
	}

	pm.logger.Info("created pattern",
		zap.String("patternID", pattern.ID),
		zap.String("name", pattern.Name),
		zap.String("severity", string(pattern.Severity)))

	return nil
}

// UpdatePattern updates an existing pattern
func (pm *PatternMatcher) UpdatePattern(ctx context.Context, patternID string, updates *ModerationPattern) error {
	// Get existing pattern
	existing, err := pm.getPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("get existing pattern: %w", err)
	}

	// Apply updates
	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	if updates.Pattern != "" {
		existing.Pattern = updates.Pattern
		// Clear regex cache
		pm.regexCache.Delete(patternID)
	}
	if updates.PatternType != "" {
		existing.PatternType = updates.PatternType
	}
	if updates.Severity != "" {
		existing.Severity = updates.Severity
	}
	if updates.Action != "" {
		existing.Action = updates.Action
	}
	if len(updates.Categories) > 0 {
		existing.Categories = updates.Categories
	}

	existing.UpdatedAt = time.Now()

	// Update in DynamoDB
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(pm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PATTERN#%s", patternID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String(`
			SET #name = :name,
			    Description = :desc,
			    Pattern = :pattern,
			    PatternType = :type,
			    Severity = :severity,
			    #action = :action,
			    UpdatedAt = :updated,
			    GSI1PK = :gsi1pk
		`),
		ExpressionAttributeNames: map[string]string{
			"#name":   "Name",
			"#action": "Action",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":name":     &types.AttributeValueMemberS{Value: existing.Name},
			":desc":     &types.AttributeValueMemberS{Value: existing.Description},
			":pattern":  &types.AttributeValueMemberS{Value: existing.Pattern},
			":type":     &types.AttributeValueMemberS{Value: existing.PatternType},
			":severity": &types.AttributeValueMemberS{Value: string(existing.Severity)},
			":action":   &types.AttributeValueMemberS{Value: string(existing.Action)},
			":updated":  &types.AttributeValueMemberS{Value: existing.UpdatedAt.Format(time.RFC3339)},
			":gsi1pk":   &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERITY#%s", existing.Severity)},
		},
	}

	_, err = pm.db.UpdateItem(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("update pattern: %w", err)
	}

	// Update cache
	pm.patterns.Store(patternID, existing)

	return nil
}

// DeletePattern deletes a pattern (soft delete by marking inactive)
func (pm *PatternMatcher) DeletePattern(ctx context.Context, patternID string) error {
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(pm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PATTERN#%s", patternID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET Active = :false, UpdatedAt = :updated"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":false":   &types.AttributeValueMemberBOOL{Value: false},
			":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}

	_, err := pm.db.UpdateItem(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("delete pattern: %w", err)
	}

	// Remove from cache
	pm.patterns.Delete(patternID)
	pm.regexCache.Delete(patternID)

	return nil
}

// GetPatterns retrieves patterns based on filter
func (pm *PatternMatcher) GetPatterns(ctx context.Context, filter PatternFilter) ([]*ModerationPattern, error) {
	patterns := []*ModerationPattern{}

	if filter.Severity != "" {
		// Query by severity using GSI
		queryInput := &dynamodb.QueryInput{
			TableName:              aws.String(pm.tableName),
			IndexName:              aws.String("GSI1"),
			KeyConditionExpression: aws.String("GSI1PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEVERITY#%s", filter.Severity)},
			},
		}

		if filter.Active != nil {
			queryInput.FilterExpression = aws.String("Active = :active")
			queryInput.ExpressionAttributeValues[":active"] = &types.AttributeValueMemberBOOL{Value: *filter.Active}
		}

		result, err := pm.db.Query(ctx, queryInput)
		if err != nil {
			return nil, fmt.Errorf("query patterns by severity: %w", err)
		}

		for _, item := range result.Items {
			pattern, err := pm.parsePattern(item)
			if err != nil {
				continue
			}
			patterns = append(patterns, pattern)
		}
	} else {
		// Scan all patterns
		scanInput := &dynamodb.ScanInput{
			TableName:        aws.String(pm.tableName),
			FilterExpression: aws.String("begins_with(PK, :prefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":prefix": &types.AttributeValueMemberS{Value: "PATTERN#"},
			},
		}

		if filter.Active != nil {
			scanInput.FilterExpression = aws.String("begins_with(PK, :prefix) AND Active = :active")
			scanInput.ExpressionAttributeValues[":active"] = &types.AttributeValueMemberBOOL{Value: *filter.Active}
		}

		result, err := pm.db.Scan(ctx, scanInput)
		if err != nil {
			return nil, fmt.Errorf("scan patterns: %w", err)
		}

		for _, item := range result.Items {
			pattern, err := pm.parsePattern(item)
			if err != nil {
				continue
			}
			patterns = append(patterns, pattern)
		}
	}

	// Apply additional filters
	filtered := patterns[:0]
	for _, pattern := range patterns {
		if pm.matchesFilter(pattern, filter) {
			filtered = append(filtered, pattern)
		}
	}

	return filtered, nil
}

// MatchContent checks content against all active patterns
func (pm *PatternMatcher) MatchContent(ctx context.Context, content string, metadata ContentMetadata) ([]PatternMatch, error) {
	matches := []PatternMatch{}
	lowerContent := strings.ToLower(content)

	// Iterate through cached patterns
	pm.patterns.Range(func(key, value interface{}) bool {
		pattern, ok := value.(*ModerationPattern)
		if !ok || !pattern.Active {
			return true
		}

		match := pm.checkPattern(pattern, content, lowerContent)
		if match != nil {
			matches = append(matches, *match)

			// Increment hit count asynchronously
			go pm.incrementHitCount(pattern.ID)
		}

		return true
	})

	// Sort matches by severity
	sortMatchesBySeverity(matches)

	return matches, nil
}

// Helper methods

func (pm *PatternMatcher) validatePattern(pattern *ModerationPattern) error {
	if pattern.Name == "" {
		return fmt.Errorf("pattern name required")
	}

	if pattern.Pattern == "" {
		return fmt.Errorf("pattern required")
	}

	// Validate regex if applicable
	if pattern.PatternType == "regex" {
		_, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
	}

	// Validate pattern type
	validTypes := map[string]bool{"regex": true, "keyword": true, "phrase": true}
	if !validTypes[pattern.PatternType] {
		return fmt.Errorf("invalid pattern type: %s", pattern.PatternType)
	}

	return nil
}

func (pm *PatternMatcher) checkPattern(pattern *ModerationPattern, content, lowerContent string) *PatternMatch {
	var matched bool
	var matchText string
	var location string

	switch pattern.PatternType {
	case "regex":
		// Get compiled regex from cache
		var regex *regexp.Regexp
		if cached, ok := pm.regexCache.Load(pattern.ID); ok {
			regex = cached.(*regexp.Regexp)
		} else {
			// Compile and cache
			var err error
			regex, err = regexp.Compile(pattern.Pattern)
			if err != nil {
				pm.logger.Warn("invalid regex pattern",
					zap.String("patternID", pattern.ID),
					zap.Error(err))
				return nil
			}
			pm.regexCache.Store(pattern.ID, regex)
		}

		if match := regex.FindStringIndex(content); match != nil {
			matched = true
			matchText = content[match[0]:match[1]]
			location = fmt.Sprintf("chars %d-%d", match[0], match[1])
		}

	case "keyword":
		keyword := strings.ToLower(pattern.Pattern)
		if idx := strings.Index(lowerContent, keyword); idx >= 0 {
			matched = true
			matchText = content[idx : idx+len(keyword)]
			location = fmt.Sprintf("char %d", idx)
		}

	case "phrase":
		phrase := strings.ToLower(pattern.Pattern)
		if strings.Contains(lowerContent, phrase) {
			matched = true
			matchText = pattern.Pattern
			location = "in content"
		}
	}

	if matched {
		return &PatternMatch{
			PatternID:   pattern.ID,
			PatternName: pattern.Name,
			MatchText:   matchText,
			Location:    location,
			Confidence:  1.0, // Pattern matches are binary
		}
	}

	return nil
}

func (pm *PatternMatcher) loadPatterns(ctx context.Context) error {
	pm.updateMutex.Lock()
	defer pm.updateMutex.Unlock()

	// Query all active patterns
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(pm.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND Active = :true"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: "PATTERN#"},
			":true":   &types.AttributeValueMemberBOOL{Value: true},
		},
	}

	result, err := pm.db.Scan(ctx, scanInput)
	if err != nil {
		return fmt.Errorf("scan patterns: %w", err)
	}

	// Clear existing patterns
	pm.patterns.Range(func(key, value interface{}) bool {
		pm.patterns.Delete(key)
		return true
	})
	pm.regexCache.Range(func(key, value interface{}) bool {
		pm.regexCache.Delete(key)
		return true
	})

	// Load new patterns
	for _, item := range result.Items {
		pattern, err := pm.parsePattern(item)
		if err != nil {
			pm.logger.Warn("failed to parse pattern", zap.Error(err))
			continue
		}

		pm.patterns.Store(pattern.ID, pattern)

		// Pre-compile regex
		if pattern.PatternType == "regex" {
			if regex, err := regexp.Compile(pattern.Pattern); err == nil {
				pm.regexCache.Store(pattern.ID, regex)
			}
		}
	}

	pm.lastUpdate = time.Now()

	pm.logger.Info("loaded patterns",
		zap.Int("count", len(result.Items)))

	return nil
}

func (pm *PatternMatcher) refreshPatternsPeriodically() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		if err := pm.loadPatterns(ctx); err != nil {
			pm.logger.Error("failed to refresh patterns", zap.Error(err))
		}
	}
}

func (pm *PatternMatcher) incrementHitCount(patternID string) {
	ctx := context.Background()

	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(pm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PATTERN#%s", patternID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("ADD HitCount :one"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one": &types.AttributeValueMemberN{Value: "1"},
		},
	}

	_, err := pm.db.UpdateItem(ctx, updateInput)
	if err != nil {
		pm.logger.Warn("failed to increment hit count",
			zap.String("patternID", patternID),
			zap.Error(err))
	}
}

func (pm *PatternMatcher) getPattern(ctx context.Context, patternID string) (*ModerationPattern, error) {
	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(pm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("PATTERN#%s", patternID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	}

	result, err := pm.db.GetItem(ctx, getInput)
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, fmt.Errorf("pattern not found")
	}

	return pm.parsePattern(result.Item)
}

func (pm *PatternMatcher) parsePattern(item map[string]types.AttributeValue) (*ModerationPattern, error) {
	pattern := &ModerationPattern{}

	// Parse fields
	if v, ok := item["ID"].(*types.AttributeValueMemberS); ok {
		pattern.ID = v.Value
	}
	if v, ok := item["Name"].(*types.AttributeValueMemberS); ok {
		pattern.Name = v.Value
	}
	if v, ok := item["Description"].(*types.AttributeValueMemberS); ok {
		pattern.Description = v.Value
	}
	if v, ok := item["Pattern"].(*types.AttributeValueMemberS); ok {
		pattern.Pattern = v.Value
	}
	if v, ok := item["PatternType"].(*types.AttributeValueMemberS); ok {
		pattern.PatternType = v.Value
	}
	if v, ok := item["Severity"].(*types.AttributeValueMemberS); ok {
		pattern.Severity = Severity(v.Value)
	}
	if v, ok := item["Action"].(*types.AttributeValueMemberS); ok {
		pattern.Action = ModerationAction(v.Value)
	}
	if v, ok := item["CreatedBy"].(*types.AttributeValueMemberS); ok {
		pattern.CreatedBy = v.Value
	}
	if v, ok := item["Active"].(*types.AttributeValueMemberBOOL); ok {
		pattern.Active = v.Value
	}
	if v, ok := item["HitCount"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%d", &pattern.HitCount)
	}

	// Parse timestamps
	if v, ok := item["CreatedAt"].(*types.AttributeValueMemberS); ok {
		pattern.CreatedAt, _ = time.Parse(time.RFC3339, v.Value)
	}
	if v, ok := item["UpdatedAt"].(*types.AttributeValueMemberS); ok {
		pattern.UpdatedAt, _ = time.Parse(time.RFC3339, v.Value)
	}

	// Parse categories
	if v, ok := item["Categories"].(*types.AttributeValueMemberL); ok {
		pattern.Categories = make([]string, 0, len(v.Value))
		for _, cat := range v.Value {
			if s, ok := cat.(*types.AttributeValueMemberS); ok {
				pattern.Categories = append(pattern.Categories, s.Value)
			}
		}
	}

	return pattern, nil
}

func (pm *PatternMatcher) matchesFilter(pattern *ModerationPattern, filter PatternFilter) bool {
	// Check categories
	if len(filter.Categories) > 0 {
		hasCategory := false
		for _, filterCat := range filter.Categories {
			for _, patternCat := range pattern.Categories {
				if filterCat == patternCat {
					hasCategory = true
					break
				}
			}
		}
		if !hasCategory {
			return false
		}
	}

	// Check created by
	if filter.CreatedBy != "" && pattern.CreatedBy != filter.CreatedBy {
		return false
	}

	return true
}

func generatePatternID(name string) string {
	// Simple ID generation
	cleaned := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	return fmt.Sprintf("%s-%d", cleaned, time.Now().Unix())
}

func sortMatchesBySeverity(matches []PatternMatch) {
	// Simple bubble sort for severity
	severityOrder := map[Severity]int{
		SeverityCritical: 4,
		SeverityHigh:     3,
		SeverityMedium:   2,
		SeverityLow:      1,
	}

	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			// Get patterns to compare severity
			// In production, you'd cache pattern info with the match
			if severityOrder[SeverityHigh] < severityOrder[SeverityMedium] {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
}
