package dynamodb

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/storage"
)

// GetStatusCount retrieves the number of statuses posted by an actor
func (s *dynamoDBStorage) GetStatusCount(ctx context.Context, actorID string) (int, error) {
	// Extract username from actorID (format: https://example.com/users/username)
	username := extractUsernameFromActorID(actorID)

	// Build the key
	pk := fmt.Sprintf("ACTOR#%s", username)
	sk := "PROFILE"

	// Get the actor profile to retrieve the status count
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ProjectionExpression: aws.String("StatusCount"),
	})
	if err != nil {
		s.logger().Error("failed to get status count",
			zap.String("actorID", actorID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to get status count: %w", err)
	}

	if result.Item == nil {
		return 0, nil // Actor not found, return 0
	}

	// Extract status count
	count := 0
	if sc, ok := result.Item["StatusCount"]; ok {
		if scNum, ok := sc.(*types.AttributeValueMemberN); ok {
			count, _ = strconv.Atoi(scNum.Value)
		}
	}

	return count, nil
}

// GetFollowingCount retrieves the exact following count for an actor
func (s *dynamoDBStorage) GetFollowingCount(ctx context.Context, actorID string) (int, error) {
	// Extract username from actorID
	username := extractUsernameFromActorID(actorID)

	// Build the key
	pk := fmt.Sprintf("ACTOR#%s", username)
	sk := "PROFILE"

	// Get the actor profile to retrieve the following count
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ProjectionExpression: aws.String("FollowingCount"),
	})
	if err != nil {
		s.logger().Error("failed to get following count",
			zap.String("actorID", actorID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to get following count: %w", err)
	}

	if result.Item == nil {
		return 0, nil // Actor not found, return 0
	}

	// Extract following count
	count := 0
	if fc, ok := result.Item["FollowingCount"]; ok {
		if fcNum, ok := fc.(*types.AttributeValueMemberN); ok {
			count, _ = strconv.Atoi(fcNum.Value)
		}
	}

	return count, nil
}

// GetFollowersCount retrieves the exact followers count for an actor (who follows you)
func (s *dynamoDBStorage) GetFollowersCount(ctx context.Context, actorID string) (int, error) {
	// Extract username from actorID
	username := extractUsernameFromActorID(actorID)

	// Build the key
	pk := fmt.Sprintf("ACTOR#%s", username)
	sk := "PROFILE"

	// Get the actor profile to retrieve the follower count
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ProjectionExpression: aws.String("FollowerCount"),
	})
	if err != nil {
		s.logger().Error("failed to get follower count",
			zap.String("actorID", actorID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to get follower count: %w", err)
	}

	if result.Item == nil {
		return 0, nil // Actor not found, return 0
	}

	// Extract follower count
	count := 0
	if fc, ok := result.Item["FollowerCount"]; ok {
		if fcNum, ok := fc.(*types.AttributeValueMemberN); ok {
			count, _ = strconv.Atoi(fcNum.Value)
		}
	}

	return count, nil
}

// GetLatestStatus retrieves the most recent status by an actor
func (s *dynamoDBStorage) GetLatestStatus(ctx context.Context, actorID string) (*storage.StatusSearchResult, error) {
	// Use GetObjectsByActor to get the most recent object
	objects, _, err := s.GetObjectsByActor(ctx, actorID, "", 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest status: %w", err)
	}

	if len(objects) == 0 {
		return nil, nil // No statuses found
	}

	// Convert the first object to a StatusSearchResult
	obj := objects[0]

	// Check if it's a Note/Article/Status
	var statusID, content, url string
	var published time.Time

	// Try to extract fields based on object type
	if objMap, ok := obj.(map[string]any); ok {
		if id, ok := objMap["id"].(string); ok {
			statusID = id
		}
		if c, ok := objMap["content"].(string); ok {
			content = c
		}
		if u, ok := objMap["url"].(string); ok {
			url = u
		}
		if p, ok := objMap["published"].(string); ok {
			published, _ = time.Parse(time.RFC3339, p)
		}

		// Extract username from actorID
		username := extractUsernameFromActorID(actorID)

		return &storage.StatusSearchResult{
			StatusID:       statusID,
			Content:        content,
			URL:            url,
			AuthorID:       actorID,
			AuthorUsername: username,
			Published:      published,
			Score:          1.0, // Default score
		}, nil
	}

	return nil, fmt.Errorf("unable to parse object as status")
}

// GetCommunityNotesByAuthor retrieves community notes authored by a specific actor
func (s *dynamoDBStorage) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	// Community notes are stored with GSI3PK = AUTHOR#<authorID>#NOTES
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s#NOTES", authorID)},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            safeInt32(limit),
	}

	// Add cursor if provided
	if cursor != "" {
		startKey, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		queryInput.ExclusiveStartKey = startKey
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		s.logger().Error("failed to query community notes by author",
			zap.String("authorID", authorID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query community notes: %w", err)
	}

	notes := make([]*storage.CommunityNote, 0, len(result.Items))
	for _, item := range result.Items {
		// Get the note ID from PK
		pk, _ := item["PK"].(*types.AttributeValueMemberS)
		if pk == nil {
			continue
		}
		noteID := pk.Value[5:] // Remove "NOTE#" prefix

		// Get full note data
		noteItem, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: s.getTableName(),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
				"SK": &types.AttributeValueMemberS{Value: "METADATA"},
			},
		})

		if err != nil || noteItem.Item == nil {
			continue
		}

		// Unmarshal the note
		var note storage.CommunityNote
		if err := attributevalue.UnmarshalMap(noteItem.Item, &note); err != nil {
			continue
		}

		notes = append(notes, &note)
	}

	// Generate next cursor
	var nextCursor string
	if result.LastEvaluatedKey != nil {
		nextCursor = encodeCursor(result.LastEvaluatedKey)
	}

	return notes, nextCursor, nil
}

// GetCommunityNoteVotes retrieves votes on a specific community note
func (s *dynamoDBStorage) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	// Query votes for the note
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			":sk": &types.AttributeValueMemberS{Value: "VOTE#"},
		},
	})
	if err != nil {
		s.logger().Error("failed to query community note votes",
			zap.String("noteID", noteID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query votes: %w", err)
	}

	votes := make([]*storage.CommunityNoteVote, 0, len(result.Items))
	for _, item := range result.Items {
		var vote storage.CommunityNoteVote
		if err := attributevalue.UnmarshalMap(item, &vote); err != nil {
			continue
		}

		// Set the Helpful flag based on VoteType for easier access
		vote.Helpful = (vote.VoteType == "helpful")

		votes = append(votes, &vote)
	}

	return votes, nil
}

// StoreReputation stores or updates a reputation record
func (s *dynamoDBStorage) StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error {
	// Extract username from actorID for key building
	username := extractUsernameFromActorID(actorID)

	// Marshal reputation to JSON for storage
	repJSON, err := json.Marshal(reputation)
	if err != nil {
		return fmt.Errorf("failed to marshal reputation: %w", err)
	}

	// Create DynamoDB item
	item := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", username)},
		"SK":             &types.AttributeValueMemberS{Value: fmt.Sprintf("REP#%s", reputation.CalculatedAt.Format(time.RFC3339))},
		"ReputationData": &types.AttributeValueMemberS{Value: string(repJSON)},
		"TotalScore":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", reputation.TotalScore)},
		"CalculatedAt":   &types.AttributeValueMemberS{Value: reputation.CalculatedAt.Format(time.RFC3339)},
		"TTL":            &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())},
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to store reputation: %w", err)
	}

	return nil
}

// GetReputation retrieves the latest reputation for an actor
func (s *dynamoDBStorage) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	// Extract username from actorID for key building
	username := extractUsernameFromActorID(actorID)

	// Query for the latest reputation record
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", username)},
			":sk_prefix": &types.AttributeValueMemberS{Value: "REP#"},
		},
		ScanIndexForward: aws.Bool(false), // Sort descending to get latest first
		Limit:            aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query reputation: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil // No reputation found
	}

	// Extract ReputationData from the item
	var item struct {
		ReputationData string `dynamodbav:"ReputationData"`
	}
	if err := attributevalue.UnmarshalMap(result.Items[0], &item); err != nil {
		return nil, fmt.Errorf("failed to unmarshal item: %w", err)
	}

	// Unmarshal the JSON reputation data
	var reputation storage.Reputation
	if err := json.Unmarshal([]byte(item.ReputationData), &reputation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reputation data: %w", err)
	}

	return &reputation, nil
}

// GetReputationHistory retrieves reputation history for an actor
func (s *dynamoDBStorage) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	// Extract username from actorID for key building
	username := extractUsernameFromActorID(actorID)

	// Query for reputation history
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", username)},
			":sk_prefix": &types.AttributeValueMemberS{Value: "REP#"},
		},
		ScanIndexForward: aws.Bool(false), // Sort descending
		Limit:            safeInt32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query reputation history: %w", err)
	}

	history := make([]*storage.Reputation, 0, len(result.Items))
	for _, item := range result.Items {
		var record struct {
			ReputationData string `dynamodbav:"ReputationData"`
		}
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			s.logger().Warn("Failed to unmarshal reputation record", zap.Error(err))
			continue
		}

		var reputation storage.Reputation
		if err := json.Unmarshal([]byte(record.ReputationData), &reputation); err != nil {
			s.logger().Warn("Failed to unmarshal reputation data", zap.Error(err))
			continue
		}

		history = append(history, &reputation)
	}

	return history, nil
}

// CreateVouch creates a new vouch
func (s *dynamoDBStorage) CreateVouch(ctx context.Context, vouch *storage.Vouch) error {
	// Generate vouch ID if not set
	if vouch.ID == "" {
		vouch.ID = fmt.Sprintf("vouch-%d-%s", time.Now().Unix(), generateRandomID(8))
	}

	// Marshal vouch to JSON
	vouchJSON, err := json.Marshal(vouch)
	if err != nil {
		return fmt.Errorf("failed to marshal vouch: %w", err)
	}

	// Create DynamoDB item
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouch.ID)},
		"SK":        &types.AttributeValueMemberS{Value: "METADATA"},
		"GSI1PK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCHER#%s", vouch.From)},
		"GSI1SK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("TO#%s", vouch.To)},
		"GSI2PK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCHEE#%s", vouch.To)},
		"GSI2SK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("FROM#%s", vouch.From)},
		"VouchData": &types.AttributeValueMemberS{Value: string(vouchJSON)},
		"Active":    &types.AttributeValueMemberBOOL{Value: vouch.Active},
		"CreatedAt": &types.AttributeValueMemberS{Value: vouch.CreatedAt.Format(time.RFC3339)},
		"ExpiresAt": &types.AttributeValueMemberS{Value: vouch.ExpiresAt.Format(time.RFC3339)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", vouch.ExpiresAt.Unix())},
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to store vouch: %w", err)
	}

	return nil
}

// GetVouch retrieves a vouch by ID
func (s *dynamoDBStorage) GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouchID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get vouch: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var item struct {
		VouchData string `dynamodbav:"VouchData"`
	}
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, fmt.Errorf("failed to unmarshal item: %w", err)
	}

	var vouch storage.Vouch
	if err := json.Unmarshal([]byte(item.VouchData), &vouch); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vouch data: %w", err)
	}

	return &vouch, nil
}

// GetVouchesByActor retrieves vouches given by an actor
func (s *dynamoDBStorage) GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCHER#%s", actorID)},
		},
	}

	if activeOnly {
		input.FilterExpression = aws.String("Active = :active")
		input.ExpressionAttributeValues[":active"] = &types.AttributeValueMemberBOOL{Value: true}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query vouches by actor: %w", err)
	}

	vouches := make([]*storage.Vouch, 0, len(result.Items))
	for _, item := range result.Items {
		var record struct {
			VouchData string `dynamodbav:"VouchData"`
		}
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			s.logger().Warn("Failed to unmarshal vouch record", zap.Error(err))
			continue
		}

		var vouch storage.Vouch
		if err := json.Unmarshal([]byte(record.VouchData), &vouch); err != nil {
			s.logger().Warn("Failed to unmarshal vouch data", zap.Error(err))
			continue
		}

		vouches = append(vouches, &vouch)
	}

	return vouches, nil
}

// GetVouchesForActor retrieves vouches received by an actor
func (s *dynamoDBStorage) GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCHEE#%s", actorID)},
		},
	}

	if activeOnly {
		input.FilterExpression = aws.String("Active = :active")
		input.ExpressionAttributeValues[":active"] = &types.AttributeValueMemberBOOL{Value: true}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query vouches for actor: %w", err)
	}

	vouches := make([]*storage.Vouch, 0, len(result.Items))
	for _, item := range result.Items {
		var record struct {
			VouchData string `dynamodbav:"VouchData"`
		}
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			s.logger().Warn("Failed to unmarshal vouch record", zap.Error(err))
			continue
		}

		var vouch storage.Vouch
		if err := json.Unmarshal([]byte(record.VouchData), &vouch); err != nil {
			s.logger().Warn("Failed to unmarshal vouch data", zap.Error(err))
			continue
		}

		vouches = append(vouches, &vouch)
	}

	return vouches, nil
}

// UpdateVouchStatus updates the active status of a vouch
func (s *dynamoDBStorage) UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	// First get the vouch to update the JSON data
	vouch, err := s.GetVouch(ctx, vouchID)
	if err != nil {
		return err
	}
	if vouch == nil {
		return fmt.Errorf("vouch not found")
	}

	// Update vouch fields
	vouch.Active = active
	vouch.Revoked = !active
	vouch.RevokedAt = revokedAt

	// Marshal updated vouch
	vouchJSON, err := json.Marshal(vouch)
	if err != nil {
		return fmt.Errorf("failed to marshal vouch: %w", err)
	}

	// Update the item
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouchID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET Active = :active, VouchData = :data"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":active": &types.AttributeValueMemberBOOL{Value: active},
			":data":   &types.AttributeValueMemberS{Value: string(vouchJSON)},
		},
	})

	return err
}

// GetMonthlyVouchCount gets the count of vouches created by an actor in a specific month
func (s *dynamoDBStorage) GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error) {
	// Calculate start and end of month
	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	// Query vouches created in the specified month
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		FilterExpression:       aws.String("CreatedAt BETWEEN :start AND :end"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCHER#%s", actorID)},
			":start": &types.AttributeValueMemberS{Value: startOfMonth.Format(time.RFC3339)},
			":end":   &types.AttributeValueMemberS{Value: endOfMonth.Format(time.RFC3339)},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to query monthly vouch count: %w", err)
	}

	return len(result.Items), nil
}
