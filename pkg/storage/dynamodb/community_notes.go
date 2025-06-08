package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/storage"
)

// CreateCommunityNote creates a new community note
func (s *dynamoDBStorage) CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error {
	// Generate ID if not provided
	if note.ID == "" {
		note.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	note.CreatedAt = now
	note.UpdatedAt = now

	// Calculate TTL (90 days)
	ttl := now.Add(90 * 24 * time.Hour).Unix()

	// Marshal note
	noteItem, err := attributevalue.MarshalMap(note)
	if err != nil {
		return fmt.Errorf("failed to marshal note: %w", err)
	}

	// Add keys
	noteItem["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", note.ID)}
	noteItem["SK"] = &types.AttributeValueMemberS{Value: "METADATA"}
	noteItem["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", ttl)}

	// GSI entries for querying
	noteItem["GSI1PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#NOTES", note.ObjectID)}
	noteItem["GSI1SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("SCORE#%010.6f#%s", note.Score, note.ID)}
	noteItem["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTES#%s", note.VisibilityStatus)}
	noteItem["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", note.CreatedAt.Format(time.RFC3339), note.ID)}
	noteItem["GSI3PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s#NOTES", note.AuthorID)}
	noteItem["GSI3SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", note.CreatedAt.Format(time.RFC3339), note.ID)}

	// Store note
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      noteItem,
	})

	if err != nil {
		return fmt.Errorf("failed to store note: %w", err)
	}

	s.logger().Debug("Created community note",
		zap.String("noteID", note.ID),
		zap.String("objectID", note.ObjectID),
		zap.String("authorID", note.AuthorID))

	return nil
}

// GetCommunityNote retrieves a note by ID
func (s *dynamoDBStorage) GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get note: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("note not found")
	}

	var note storage.CommunityNote
	if err := attributevalue.UnmarshalMap(result.Item, &note); err != nil {
		return nil, fmt.Errorf("failed to unmarshal note: %w", err)
	}

	return &note, nil
}

// GetVisibleCommunityNotes retrieves visible notes for an object
func (s *dynamoDBStorage) GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error) {
	// Query by object ID and score
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#NOTES", objectID)},
		},
		ScanIndexForward: aws.Bool(false), // Descending order by score
		Limit:            aws.Int32(50),   // Limit to top 50 notes
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}

	notes := make([]*storage.CommunityNote, 0, len(result.Items))
	for _, item := range result.Items {
		// Get full note data
		pk, _ := item["PK"].(*types.AttributeValueMemberS)
		if pk == nil {
			continue
		}
		noteID := pk.Value[5:] // Remove "NOTE#" prefix

		note, err := s.GetCommunityNote(ctx, noteID)
		if err != nil {
			continue
		}

		// Only include visible notes
		if note.VisibilityStatus == "visible" || note.VisibilityStatus == "prominent" {
			notes = append(notes, note)
		}
	}

	return notes, nil
}

// UpdateCommunityNoteScore updates a note's score and visibility
func (s *dynamoDBStorage) UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error {
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET Score = :score, VisibilityStatus = :status, UpdatedAt = :updated"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":score":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%f", score)},
			":status":  &types.AttributeValueMemberS{Value: status},
			":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	})

	return err
}

// CreateCommunityNoteVote creates a vote on a note
func (s *dynamoDBStorage) CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error {
	vote.CreatedAt = time.Now()

	voteItem, err := attributevalue.MarshalMap(vote)
	if err != nil {
		return fmt.Errorf("failed to marshal vote: %w", err)
	}

	// Add keys
	voteItem["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", vote.NoteID)}
	voteItem["SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("VOTE#%s", vote.VoterID)}
	voteItem["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix())}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      voteItem,
	})

	return err
}

// GetUserCommunityNoteVotes retrieves a user's votes on specific notes
func (s *dynamoDBStorage) GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	votes := make(map[string]*storage.CommunityNoteVote)

	// Batch get votes
	keys := make([]map[string]types.AttributeValue, 0, len(noteIDs))
	for _, noteID := range noteIDs {
		keys = append(keys, map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOTE#%s", userID)},
		})
	}

	// DynamoDB limits batch gets to 100 items
	for i := 0; i < len(keys); i += 100 {
		end := i + 100
		if end > len(keys) {
			end = len(keys)
		}

		result, err := s.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				*s.getTableName(): {
					Keys: keys[i:end],
				},
			},
		})

		if err != nil {
			return nil, fmt.Errorf("failed to batch get votes: %w", err)
		}

		for _, item := range result.Responses[*s.getTableName()] {
			var vote storage.CommunityNoteVote
			if err := attributevalue.UnmarshalMap(item, &vote); err != nil {
				continue
			}
			votes[vote.NoteID] = &vote
		}
	}

	return votes, nil
}

// CheckCommunityNoteRateLimit checks if a user can create more notes today
func (s *dynamoDBStorage) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	// Query notes created by user in last 24 hours
	yesterday := time.Now().Add(-24 * time.Hour)

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk AND GSI3SK > :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s#NOTES", userID)},
			":sk": &types.AttributeValueMemberS{Value: yesterday.Format(time.RFC3339)},
		},
		Select: types.SelectCount,
	})

	if err != nil {
		return false, 0, err
	}

	count := int(result.Count)
	canCreate := count < limit
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	return canCreate, remaining, nil
}
