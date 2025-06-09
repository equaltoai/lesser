package streaming

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// SessionManager manages streaming sessions
type SessionManager struct {
	db          *dynamodb.Client
	tableName   string
	logger      *zap.Logger
	costTracker CostTracker
}

// NewSessionManager creates a new session manager
func NewSessionManager(db *dynamodb.Client, tableName string, logger *zap.Logger, costTracker CostTracker) *SessionManager {
	return &SessionManager{
		db:          db,
		tableName:   tableName,
		logger:      logger,
		costTracker: costTracker,
	}
}

// CreateSession creates a new streaming session
func (sm *SessionManager) CreateSession(ctx context.Context, session *StreamingSession) error {
	item := map[string]types.AttributeValue{
		"PK":               &types.AttributeValueMemberS{Value: fmt.Sprintf("SESSION#%s", session.SessionID)},
		"SK":               &types.AttributeValueMemberS{Value: "METADATA"},
		"UserID":           &types.AttributeValueMemberS{Value: session.UserID},
		"MediaID":          &types.AttributeValueMemberS{Value: session.MediaID},
		"Format":           &types.AttributeValueMemberS{Value: string(session.Format)},
		"CurrentQuality":   &types.AttributeValueMemberS{Value: string(session.CurrentQuality)},
		"StartTime":        &types.AttributeValueMemberS{Value: session.StartTime.Format(time.RFC3339)},
		"LastSegmentIndex": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", session.LastSegmentIndex)},
		"BytesTransferred": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", session.BytesTransferred)},
		"BufferHealth":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", session.BufferHealth)},
		"Active":           &types.AttributeValueMemberBOOL{Value: true},
		"TTL":              &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(24*time.Hour).Unix())},

		// GSI for user sessions
		"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", session.UserID)},
		"GSI1SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SESSION#%s", session.StartTime.Format(time.RFC3339))},
	}

	putInput := &dynamodb.PutItemInput{
		TableName:           aws.String(sm.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	_, err := sm.db.PutItem(ctx, putInput)
	if err != nil {
		sm.logger.Error("failed to create session",
			zap.String("sessionID", session.SessionID),
			zap.Error(err))
		return fmt.Errorf("create session: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoWrite(1)
	}

	return nil
}

// GetSession retrieves a streaming session
func (sm *SessionManager) GetSession(ctx context.Context, sessionID string) (*StreamingSession, error) {
	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(sm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SESSION#%s", sessionID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	}

	result, err := sm.db.GetItem(ctx, getInput)
	if err != nil {
		sm.logger.Error("failed to get session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoRead(1)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return sm.parseSession(result.Item)
}

// UpdateSession updates a streaming session
func (sm *SessionManager) UpdateSession(ctx context.Context, session *StreamingSession) error {
	// First update all the main attributes
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(sm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SESSION#%s", session.SessionID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String(`
			SET CurrentQuality = :quality,
			    LastSegmentIndex = :segment,
			    BytesTransferred = :bytes,
			    BufferHealth = :buffer,
			    LastUpdate = :timestamp
		`),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":quality":   &types.AttributeValueMemberS{Value: string(session.CurrentQuality)},
			":segment":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", session.LastSegmentIndex)},
			":bytes":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", session.BytesTransferred)},
			":buffer":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", session.BufferHealth)},
			":timestamp": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	}

	_, err := sm.db.UpdateItem(ctx, updateInput)
	if err != nil {
		sm.logger.Error("failed to update session",
			zap.String("sessionID", session.SessionID),
			zap.Error(err))
		return fmt.Errorf("update session: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoWrite(1)
	}

	// Log quality change if it occurred
	sm.trackQualityChange(ctx, session)

	return nil
}

// EndSession marks a session as ended
func (sm *SessionManager) EndSession(ctx context.Context, sessionID string) error {
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(sm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SESSION#%s", sessionID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String(`
			SET Active = :false,
			    EndTime = :endtime,
			    Duration = :duration
		`),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":false":    &types.AttributeValueMemberBOOL{Value: false},
			":endtime":  &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
			":duration": &types.AttributeValueMemberN{Value: "0"}, // Will be calculated below
		},
		ReturnValues: types.ReturnValueAllNew,
	}

	result, err := sm.db.UpdateItem(ctx, updateInput)
	if err != nil {
		sm.logger.Error("failed to end session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return fmt.Errorf("end session: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoWrite(1)
	}

	// Calculate and update duration
	if startTimeAttr, ok := result.Attributes["StartTime"]; ok {
		if startTimeStr, ok := startTimeAttr.(*types.AttributeValueMemberS); ok {
			startTime, _ := time.Parse(time.RFC3339, startTimeStr.Value)
			duration := time.Since(startTime).Seconds()

			// Update duration
			sm.updateSessionDuration(ctx, sessionID, duration)
		}
	}

	return nil
}

// GetUserSessions retrieves active sessions for a user
func (sm *SessionManager) GetUserSessions(ctx context.Context, userID string) ([]*StreamingSession, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(sm.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":   &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			":true": &types.AttributeValueMemberBOOL{Value: true},
		},
		FilterExpression: aws.String("Active = :true"),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	result, err := sm.db.Query(ctx, queryInput)
	if err != nil {
		sm.logger.Error("failed to query user sessions",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, fmt.Errorf("query user sessions: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoRead(len(result.Items))
	}

	sessions := make([]*StreamingSession, 0, len(result.Items))
	for _, item := range result.Items {
		session, err := sm.parseSession(item)
		if err != nil {
			sm.logger.Warn("failed to parse session",
				zap.Error(err))
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// GetMediaSessions retrieves sessions for a specific media item
func (sm *SessionManager) GetMediaSessions(ctx context.Context, mediaID string, limit int32) ([]*StreamingSession, error) {
	// This would require another GSI in production
	// For now, we'll scan with a filter (not efficient for large datasets)
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(sm.tableName),
		FilterExpression: aws.String("MediaID = :media AND Active = :true"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":media": &types.AttributeValueMemberS{Value: mediaID},
			":true":  &types.AttributeValueMemberBOOL{Value: true},
		},
		Limit: aws.Int32(limit),
	}

	result, err := sm.db.Scan(ctx, scanInput)
	if err != nil {
		sm.logger.Error("failed to scan media sessions",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, fmt.Errorf("scan media sessions: %w", err)
	}

	// Track cost
	if sm.costTracker != nil {
		sm.costTracker.TrackDynamoRead(len(result.Items))
	}

	sessions := make([]*StreamingSession, 0, len(result.Items))
	for _, item := range result.Items {
		session, err := sm.parseSession(item)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// Helper methods

func (sm *SessionManager) parseSession(item map[string]types.AttributeValue) (*StreamingSession, error) {
	session := &StreamingSession{}

	// Parse session ID from PK
	if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
		session.SessionID = pk.Value[8:] // Remove "SESSION#" prefix
	}

	// Parse other fields
	if v, ok := item["UserID"].(*types.AttributeValueMemberS); ok {
		session.UserID = v.Value
	}
	if v, ok := item["MediaID"].(*types.AttributeValueMemberS); ok {
		session.MediaID = v.Value
	}
	if v, ok := item["Format"].(*types.AttributeValueMemberS); ok {
		session.Format = MediaFormat(v.Value)
	}
	if v, ok := item["CurrentQuality"].(*types.AttributeValueMemberS); ok {
		session.CurrentQuality = Quality(v.Value)
	}
	if v, ok := item["StartTime"].(*types.AttributeValueMemberS); ok {
		session.StartTime, _ = time.Parse(time.RFC3339, v.Value)
	}
	if v, ok := item["LastSegmentIndex"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &session.LastSegmentIndex); err != nil {
			sm.logger.Warn("failed to parse LastSegmentIndex", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := item["BytesTransferred"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &session.BytesTransferred); err != nil {
			sm.logger.Warn("failed to parse BytesTransferred", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := item["BufferHealth"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%f", &session.BufferHealth); err != nil {
			sm.logger.Warn("failed to parse BufferHealth", zap.String("value", v.Value), zap.Error(err))
		}
	}

	return session, nil
}

func (sm *SessionManager) updateSessionDuration(ctx context.Context, sessionID string, duration float64) {
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(sm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SESSION#%s", sessionID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET Duration = :duration"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":duration": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", duration)},
		},
	}

	_, err := sm.db.UpdateItem(ctx, updateInput)
	if err != nil {
		sm.logger.Warn("failed to update session duration",
			zap.String("sessionID", sessionID),
			zap.Error(err))
	}
}

func (sm *SessionManager) trackQualityChange(ctx context.Context, session *StreamingSession) {
	// Track quality changes for analytics
	putInput := &dynamodb.PutItemInput{
		TableName: aws.String("lesser-analytics"),
		Item: map[string]types.AttributeValue{
			"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("QUALITY#%s", session.SessionID)},
			"SK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", time.Now().UnixNano())},
			"Quality":   &types.AttributeValueMemberS{Value: string(session.CurrentQuality)},
			"Timestamp": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
			"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(7*24*time.Hour).Unix())},
		},
	}

	_, err := sm.db.PutItem(ctx, putInput)
	if err != nil {
		sm.logger.Warn("failed to track quality change",
			zap.Error(err))
	}
}

// CleanupExpiredSessions removes sessions older than the specified duration
func (sm *SessionManager) CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)

	// This would be more efficient with a GSI on timestamp
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(sm.tableName),
		FilterExpression: aws.String("StartTime < :cutoff AND Active = :false"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":cutoff": &types.AttributeValueMemberS{Value: cutoff.Format(time.RFC3339)},
			":false":  &types.AttributeValueMemberBOOL{Value: false},
		},
	}

	result, err := sm.db.Scan(ctx, scanInput)
	if err != nil {
		return fmt.Errorf("scan expired sessions: %w", err)
	}

	// Delete expired sessions
	for _, item := range result.Items {
		if pk, ok := item["PK"]; ok {
			deleteInput := &dynamodb.DeleteItemInput{
				TableName: aws.String(sm.tableName),
				Key: map[string]types.AttributeValue{
					"PK": pk,
					"SK": &types.AttributeValueMemberS{Value: "METADATA"},
				},
			}

			_, err := sm.db.DeleteItem(ctx, deleteInput)
			if err != nil {
				sm.logger.Warn("failed to delete expired session",
					zap.Error(err))
			}
		}
	}

	sm.logger.Info("cleaned up expired sessions",
		zap.Int("count", len(result.Items)))

	return nil
}
