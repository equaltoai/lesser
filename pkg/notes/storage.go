package notes

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var (
	dynamoClient *dynamodb.Client
	tableName    = aws.String("lesser-main")
)

// SetDynamoClient allows external packages to set the DynamoDB client
func SetDynamoClient(client *dynamodb.Client) {
	dynamoClient = client
}

// StoreNote stores a community note in DynamoDB
func StoreNote(ctx context.Context, note *CommunityNote) error {
	// Set TTL
	note.TTL = time.Now().Add(NoteTTLDays * 24 * time.Hour).Unix()

	// Store main note entry
	noteItem, err := attributevalue.MarshalMap(note)
	if err != nil {
		return fmt.Errorf("failed to marshal note: %w", err)
	}

	// Add keys
	noteItem["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", note.ID)}
	noteItem["SK"] = &types.AttributeValueMemberS{Value: "METADATA"}

	// GSI entries for querying
	noteItem["GSI1PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s#NOTES", note.ObjectID)}
	noteItem["GSI1SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("SCORE#%010.6f#%s", note.Score, note.ID)}
	noteItem["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTES#%s", note.VisibilityStatus)}
	noteItem["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", note.CreatedAt.Format(time.RFC3339), note.ID)}
	noteItem["GSI3PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s#NOTES", note.AuthorID)}
	noteItem["GSI3SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", note.CreatedAt.Format(time.RFC3339), note.ID)}

	// Store main note
	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: tableName,
		Item:      noteItem,
	})
	if err != nil {
		return fmt.Errorf("failed to store note: %w", err)
	}

	// Store object index entry
	objectIndexItem := map[string]types.AttributeValue{
		"PK":  &types.AttributeValueMemberS{Value: fmt.Sprintf("OBJECT#%s", note.ObjectID)},
		"SK":  &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", note.ID)},
		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", note.TTL)},
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: tableName,
		Item:      objectIndexItem,
	})

	return err
}

// GetNote retrieves a note by ID
func GetNote(ctx context.Context, noteID string) (*CommunityNote, error) {
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: tableName,
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

	var note CommunityNote
	if err := attributevalue.UnmarshalMap(result.Item, &note); err != nil {
		return nil, fmt.Errorf("failed to unmarshal note: %w", err)
	}

	return &note, nil
}

// GetVisibleNotes retrieves visible notes for an object
func GetVisibleNotes(ctx context.Context, objectID string) ([]CommunityNote, error) {
	// Query by object ID and score
	result, err := dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:              tableName,
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

	notes := make([]CommunityNote, 0, len(result.Items))
	for _, item := range result.Items {
		// Get full note data
		pk, _ := item["PK"].(*types.AttributeValueMemberS)
		noteID := pk.Value[5:] // Remove "NOTE#" prefix

		note, err := GetNote(ctx, noteID)
		if err != nil {
			continue
		}

		// Only include visible notes
		if note.VisibilityStatus == VisibilityVisible || note.VisibilityStatus == VisibilityProminent {
			notes = append(notes, *note)
		}
	}

	return notes, nil
}

// StoreVote stores a vote on a note
func StoreVote(ctx context.Context, vote *Vote) error {
	voteItem, err := attributevalue.MarshalMap(vote)
	if err != nil {
		return fmt.Errorf("failed to marshal vote: %w", err)
	}

	// Add keys
	voteItem["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", vote.NoteID)}
	voteItem["SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("VOTE#%s", vote.VoterID)}
	voteItem["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(NoteTTLDays*24*time.Hour).Unix())}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: tableName,
		Item:      voteItem,
	})

	return err
}

// GetVotesForNote retrieves all votes for a note
func GetVotesForNote(ctx context.Context, noteID string) ([]Vote, error) {
	result, err := dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:              tableName,
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			":sk": &types.AttributeValueMemberS{Value: "VOTE#"},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query votes: %w", err)
	}

	votes := make([]Vote, 0, len(result.Items))
	for _, item := range result.Items {
		var vote Vote
		if err := attributevalue.UnmarshalMap(item, &vote); err != nil {
			continue
		}
		votes = append(votes, vote)
	}

	return votes, nil
}

// GetUserVotes retrieves a user's votes on specific notes
func GetUserVotes(ctx context.Context, userID string, noteIDs []string) (map[string]Vote, error) {
	votes := make(map[string]Vote)

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

		result, err := dynamoClient.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				*tableName: {
					Keys: keys[i:end],
				},
			},
		})

		if err != nil {
			return nil, fmt.Errorf("failed to batch get votes: %w", err)
		}

		for _, item := range result.Responses[*tableName] {
			var vote Vote
			if err := attributevalue.UnmarshalMap(item, &vote); err != nil {
				continue
			}
			votes[vote.NoteID] = vote
		}
	}

	return votes, nil
}

// UpdateNoteScore updates a note's score and visibility
func UpdateNoteScore(ctx context.Context, noteID string, score float64, status VisibilityStatus) error {
	_, err := dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET Score = :score, VisibilityStatus = :status, UpdatedAt = :updated"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":score":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%f", score)},
			":status":  &types.AttributeValueMemberS{Value: string(status)},
			":updated": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	})

	return err
}

// UpdateNoteAnalysis updates AI analysis results
func UpdateNoteAnalysis(ctx context.Context, noteID string, analysis *Analysis, sourceQuality float64) error {
	_, err := dynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: tableName,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("NOTE#%s", noteID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET Sentiment = :sentiment, Objectivity = :objectivity, SourceQuality = :quality, UpdatedAt = :updated"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sentiment":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%f", analysis.Sentiment)},
			":objectivity": &types.AttributeValueMemberN{Value: fmt.Sprintf("%f", analysis.Objectivity)},
			":quality":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%f", sourceQuality)},
			":updated":     &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	})

	return err
}

// GetNotesByAuthor retrieves notes created by a specific author
func GetNotesByAuthor(ctx context.Context, authorID string, limit int32) ([]CommunityNote, error) {
	result, err := dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:              tableName,
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s#NOTES", authorID)},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(limit),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query author notes: %w", err)
	}

	notes := make([]CommunityNote, 0, len(result.Items))
	for _, item := range result.Items {
		// Get full note data
		pk, _ := item["PK"].(*types.AttributeValueMemberS)
		noteID := pk.Value[5:] // Remove "NOTE#" prefix

		note, err := GetNote(ctx, noteID)
		if err != nil {
			continue
		}

		notes = append(notes, *note)
	}

	return notes, nil
}

// CheckNoteRateLimit checks if a user can create more notes today
func CheckNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int) {
	// Query notes created by user in last 24 hours
	yesterday := time.Now().Add(-24 * time.Hour)

	result, err := dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:              tableName,
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk AND GSI3SK > :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s#NOTES", userID)},
			":sk": &types.AttributeValueMemberS{Value: yesterday.Format(time.RFC3339)},
		},
		Select: types.SelectCount,
	})

	if err != nil {
		return false, 0
	}

	count := int(result.Count)
	return count < limit, limit - count
}

// GenerateNoteID generates a unique ID for a note
func GenerateNoteID() string {
	return uuid.New().String()
}

// Constants for visibility status
const (
	VisibilityProminent VisibilityStatus = "prominent" // High-scoring notes
)
