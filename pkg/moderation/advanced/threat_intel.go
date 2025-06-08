package advanced

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// ThreatIntelligence manages cross-instance threat sharing
type ThreatIntelligence struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger

	// Cache for active threats
	threatCache sync.Map
	lastUpdate  time.Time
	updateMutex sync.RWMutex
}

// NewThreatIntelligence creates a new threat intelligence component
func NewThreatIntelligence(db *dynamodb.Client, tableName string, logger *zap.Logger) *ThreatIntelligence {
	ti := &ThreatIntelligence{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}

	// Load threats on initialization
	ctx := context.Background()
	if err := ti.loadThreats(ctx); err != nil {
		logger.Warn("failed to load threats on init", zap.Error(err))
	}

	// Start periodic refresh
	go ti.refreshThreatsPeriodically()

	return ti
}

// ShareThreat shares a new threat with the network
func (ti *ThreatIntelligence) ShareThreat(ctx context.Context, threat *ThreatIntel) error {
	// Validate threat
	if err := ti.validateThreat(threat); err != nil {
		return fmt.Errorf("invalid threat: %w", err)
	}

	// Generate ID if not provided
	if threat.ID == "" {
		threat.ID = ti.generateThreatID(threat)
	}

	// Set timestamps
	now := time.Now()
	if threat.FirstSeen.IsZero() {
		threat.FirstSeen = now
	}
	threat.LastSeen = now

	// Store in DynamoDB
	item := map[string]types.AttributeValue{
		"PK":           &types.AttributeValueMemberS{Value: fmt.Sprintf("THREAT#%s", threat.ID)},
		"SK":           &types.AttributeValueMemberS{Value: "METADATA"},
		"ID":           &types.AttributeValueMemberS{Value: threat.ID},
		"ThreatType":   &types.AttributeValueMemberS{Value: threat.ThreatType},
		"Severity":     &types.AttributeValueMemberS{Value: string(threat.Severity)},
		"Description":  &types.AttributeValueMemberS{Value: threat.Description},
		"SourceDomain": &types.AttributeValueMemberS{Value: threat.SourceDomain},
		"FirstSeen":    &types.AttributeValueMemberS{Value: threat.FirstSeen.Format(time.RFC3339)},
		"LastSeen":     &types.AttributeValueMemberS{Value: threat.LastSeen.Format(time.RFC3339)},
		"HitCount":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", threat.HitCount)},
		"Confidence":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", threat.Confidence)},
		"TTL":          &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", now.Add(threat.TTL).Unix())},

		// GSI for querying by type
		"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TYPE#%s", threat.ThreatType)},
		"GSI1SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("THREAT#%s", threat.ID)},

		// GSI for querying by time
		"GSI2PK": &types.AttributeValueMemberS{Value: "THREATS"},
		"GSI2SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%d#%s", threat.LastSeen.Unix(), threat.ID)},
	}

	// Add indicators
	if len(threat.Indicators) > 0 {
		indicatorList := &types.AttributeValueMemberL{
			Value: make([]types.AttributeValue, len(threat.Indicators)),
		}
		for i, indicator := range threat.Indicators {
			indicatorList.Value[i] = &types.AttributeValueMemberS{Value: indicator}
		}
		item["Indicators"] = indicatorList
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(ti.tableName),
		Item:      item,
	}

	_, err := ti.db.PutItem(ctx, putInput)
	if err != nil {
		return fmt.Errorf("share threat: %w", err)
	}

	// Update cache
	ti.threatCache.Store(threat.ID, threat)

	// Store indicators for fast lookup
	for _, indicator := range threat.Indicators {
		ti.storeIndicator(ctx, indicator, threat.ID)
	}

	ti.logger.Info("shared threat",
		zap.String("threatID", threat.ID),
		zap.String("type", threat.ThreatType),
		zap.String("severity", string(threat.Severity)),
		zap.Int("indicators", len(threat.Indicators)))

	return nil
}

// GetSharedThreats retrieves threats shared since a given time
func (ti *ThreatIntelligence) GetSharedThreats(ctx context.Context, since time.Time) ([]*ThreatIntel, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(ti.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk AND GSI2SK > :since"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: "THREATS"},
			":since": &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", since.Unix())},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(100),  // Limit to recent threats
	}

	result, err := ti.db.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("query threats: %w", err)
	}

	threats := make([]*ThreatIntel, 0, len(result.Items))
	for _, item := range result.Items {
		threat, err := ti.parseThreat(item)
		if err != nil {
			ti.logger.Warn("failed to parse threat", zap.Error(err))
			continue
		}
		threats = append(threats, threat)
	}

	return threats, nil
}

// CheckContent checks content against known threats
func (ti *ThreatIntelligence) CheckContent(ctx context.Context, content string, metadata ContentMetadata) ([]ThreatMatch, error) {
	matches := []ThreatMatch{}
	lowerContent := strings.ToLower(content)

	// Check against cached threats
	ti.threatCache.Range(func(key, value interface{}) bool {
		threat, ok := value.(*ThreatIntel)
		if !ok {
			return true
		}

		// Check each indicator
		for _, indicator := range threat.Indicators {
			if ti.matchesIndicator(indicator, content, lowerContent, metadata) {
				matches = append(matches, ThreatMatch{
					ThreatID:   threat.ID,
					ThreatType: threat.ThreatType,
					Indicator:  indicator,
					Confidence: threat.Confidence,
				})

				// Increment hit count asynchronously
				go ti.incrementHitCount(threat.ID)
			}
		}

		return true
	})

	// Check URLs against threat domains
	for _, url := range metadata.URLs {
		if threatID := ti.checkURLThreat(url); threatID != "" {
			matches = append(matches, ThreatMatch{
				ThreatID:   threatID,
				ThreatType: "malicious_url",
				Indicator:  url,
				Confidence: 0.9,
			})
		}
	}

	// Check for hash matches (for images/files)
	if metadata.ContentType == ContentTypeImage {
		hash := ti.hashContent(content)
		if threatID := ti.checkHashThreat(hash); threatID != "" {
			matches = append(matches, ThreatMatch{
				ThreatID:   threatID,
				ThreatType: "malicious_content",
				Indicator:  hash,
				Confidence: 1.0,
			})
		}
	}

	return matches, nil
}

// GetThreatsByType retrieves threats of a specific type
func (ti *ThreatIntelligence) GetThreatsByType(ctx context.Context, threatType string, limit int) ([]*ThreatIntel, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(ti.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TYPE#%s", threatType)},
		},
		Limit: aws.Int32(int32(limit)),
	}

	result, err := ti.db.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("query threats by type: %w", err)
	}

	threats := make([]*ThreatIntel, 0, len(result.Items))
	for _, item := range result.Items {
		threat, err := ti.parseThreat(item)
		if err != nil {
			continue
		}
		threats = append(threats, threat)
	}

	return threats, nil
}

// UpdateThreatConfidence updates the confidence score of a threat
func (ti *ThreatIntelligence) UpdateThreatConfidence(ctx context.Context, threatID string, newConfidence float64) error {
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(ti.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("THREAT#%s", threatID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET Confidence = :conf, LastSeen = :seen"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":conf": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", newConfidence)},
			":seen": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}

	_, err := ti.db.UpdateItem(ctx, updateInput)
	return err
}

// Helper methods

func (ti *ThreatIntelligence) validateThreat(threat *ThreatIntel) error {
	if threat.ThreatType == "" {
		return fmt.Errorf("threat type required")
	}

	if len(threat.Indicators) == 0 {
		return fmt.Errorf("at least one indicator required")
	}

	if threat.Severity == "" {
		threat.Severity = SeverityMedium
	}

	if threat.Confidence == 0 {
		threat.Confidence = 0.7
	}

	if threat.TTL == 0 {
		threat.TTL = 7 * 24 * time.Hour // Default 7 days
	}

	return nil
}

func (ti *ThreatIntelligence) generateThreatID(threat *ThreatIntel) string {
	// Generate ID based on threat type and indicators
	h := sha256.New()
	h.Write([]byte(threat.ThreatType))
	for _, indicator := range threat.Indicators {
		h.Write([]byte(indicator))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func (ti *ThreatIntelligence) matchesIndicator(indicator, content, lowerContent string, metadata ContentMetadata) bool {
	// Simple string matching - in production, use more sophisticated matching
	indicatorLower := strings.ToLower(indicator)

	// Check content
	if strings.Contains(lowerContent, indicatorLower) {
		return true
	}

	// Check URLs
	for _, url := range metadata.URLs {
		if strings.Contains(strings.ToLower(url), indicatorLower) {
			return true
		}
	}

	// Check hashtags
	for _, tag := range metadata.Hashtags {
		if strings.EqualFold(tag, indicator) {
			return true
		}
	}

	return false
}

func (ti *ThreatIntelligence) checkURLThreat(url string) string {
	// Check if URL matches known malicious domains
	// This is simplified - in production, use proper URL parsing and domain extraction
	maliciousDomains := []string{
		"malicious.com",
		"phishing-site.net",
		"scam-domain.org",
	}

	urlLower := strings.ToLower(url)
	for _, domain := range maliciousDomains {
		if strings.Contains(urlLower, domain) {
			return "known-malicious-domain"
		}
	}

	return ""
}

func (ti *ThreatIntelligence) checkHashThreat(hash string) string {
	// Check if hash matches known malicious content
	// In production, query the database for hash matches
	return ""
}

func (ti *ThreatIntelligence) hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

func (ti *ThreatIntelligence) storeIndicator(ctx context.Context, indicator, threatID string) {
	// Store indicator for fast lookup
	item := map[string]types.AttributeValue{
		"PK":       &types.AttributeValueMemberS{Value: fmt.Sprintf("INDICATOR#%s", indicator)},
		"SK":       &types.AttributeValueMemberS{Value: fmt.Sprintf("THREAT#%s", threatID)},
		"ThreatID": &types.AttributeValueMemberS{Value: threatID},
		"TTL":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(30*24*time.Hour).Unix())},
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(ti.tableName),
		Item:      item,
	}

	_, err := ti.db.PutItem(ctx, putInput)
	if err != nil {
		ti.logger.Warn("failed to store indicator",
			zap.String("indicator", indicator),
			zap.Error(err))
	}
}

func (ti *ThreatIntelligence) incrementHitCount(threatID string) {
	ctx := context.Background()

	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(ti.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("THREAT#%s", threatID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("ADD HitCount :one SET LastSeen = :seen"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one":  &types.AttributeValueMemberN{Value: "1"},
			":seen": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}

	_, err := ti.db.UpdateItem(ctx, updateInput)
	if err != nil {
		ti.logger.Warn("failed to increment hit count",
			zap.String("threatID", threatID),
			zap.Error(err))
	}
}

func (ti *ThreatIntelligence) loadThreats(ctx context.Context) error {
	ti.updateMutex.Lock()
	defer ti.updateMutex.Unlock()

	// Clear existing threats
	ti.threatCache.Range(func(key, value interface{}) bool {
		ti.threatCache.Delete(key)
		return true
	})

	// Load active threats (not expired)
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(ti.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND #ttl > :now"),
		ExpressionAttributeNames: map[string]string{
			"#ttl": "TTL",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: "THREAT#"},
			":now":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
		},
	}

	result, err := ti.db.Scan(ctx, scanInput)
	if err != nil {
		return fmt.Errorf("scan threats: %w", err)
	}

	for _, item := range result.Items {
		threat, err := ti.parseThreat(item)
		if err != nil {
			ti.logger.Warn("failed to parse threat", zap.Error(err))
			continue
		}

		ti.threatCache.Store(threat.ID, threat)
	}

	ti.lastUpdate = time.Now()
	ti.logger.Info("loaded threats", zap.Int("count", len(result.Items)))

	return nil
}

func (ti *ThreatIntelligence) refreshThreatsPeriodically() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		if err := ti.loadThreats(ctx); err != nil {
			ti.logger.Error("failed to refresh threats", zap.Error(err))
		}
	}
}

func (ti *ThreatIntelligence) parseThreat(item map[string]types.AttributeValue) (*ThreatIntel, error) {
	threat := &ThreatIntel{}

	// Parse fields
	if v, ok := item["ID"].(*types.AttributeValueMemberS); ok {
		threat.ID = v.Value
	}
	if v, ok := item["ThreatType"].(*types.AttributeValueMemberS); ok {
		threat.ThreatType = v.Value
	}
	if v, ok := item["Severity"].(*types.AttributeValueMemberS); ok {
		threat.Severity = Severity(v.Value)
	}
	if v, ok := item["Description"].(*types.AttributeValueMemberS); ok {
		threat.Description = v.Value
	}
	if v, ok := item["SourceDomain"].(*types.AttributeValueMemberS); ok {
		threat.SourceDomain = v.Value
	}
	if v, ok := item["HitCount"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%d", &threat.HitCount)
	}
	if v, ok := item["Confidence"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(v.Value, "%f", &threat.Confidence)
	}

	// Parse timestamps
	if v, ok := item["FirstSeen"].(*types.AttributeValueMemberS); ok {
		threat.FirstSeen, _ = time.Parse(time.RFC3339, v.Value)
	}
	if v, ok := item["LastSeen"].(*types.AttributeValueMemberS); ok {
		threat.LastSeen, _ = time.Parse(time.RFC3339, v.Value)
	}

	// Parse TTL
	if v, ok := item["TTL"].(*types.AttributeValueMemberN); ok {
		var ttlUnix int64
		fmt.Sscanf(v.Value, "%d", &ttlUnix)
		threat.TTL = time.Until(time.Unix(ttlUnix, 0))
	}

	// Parse indicators
	if v, ok := item["Indicators"].(*types.AttributeValueMemberL); ok {
		threat.Indicators = make([]string, 0, len(v.Value))
		for _, indicator := range v.Value {
			if s, ok := indicator.(*types.AttributeValueMemberS); ok {
				threat.Indicators = append(threat.Indicators, s.Value)
			}
		}
	}

	return threat, nil
}
