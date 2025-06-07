package dynamodb

import (
	"context"
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

// GetFollowerCount retrieves the exact follower count for an actor
func (s *dynamoDBStorage) GetFollowerCount(ctx context.Context, actorID string) (int, error) {
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
	if objMap, ok := obj.(map[string]interface{}); ok {
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
		Limit:            aws.Int32(int32(limit)),
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
