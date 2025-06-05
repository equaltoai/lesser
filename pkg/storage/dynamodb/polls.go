package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// Poll represents a poll stored in DynamoDB
type Poll struct {
	ID          string           `dynamodbav:"id"`
	StatusID    string           `dynamodbav:"statusId"`
	CreatedBy   string           `dynamodbav:"createdBy"`
	Options     []string         `dynamodbav:"options"`
	Multiple    bool             `dynamodbav:"multiple"`
	HideTotals  bool             `dynamodbav:"hideTotals"`
	ExpiresAt   time.Time        `dynamodbav:"expiresAt"`
	CreatedAt   time.Time        `dynamodbav:"createdAt"`
	UpdatedAt   time.Time        `dynamodbav:"updatedAt"`
	VotesCount  int              `dynamodbav:"votesCount"`
	VotersCount int              `dynamodbav:"votersCount"`
	Votes       map[string][]int `dynamodbav:"votes"` // Map of voter ID to option indices
}

// PollRecord represents a poll record in DynamoDB
type PollRecord struct {
	PK        string    `dynamodbav:"PK"`            // POLL#{pollId}
	SK        string    `dynamodbav:"SK"`            // METADATA
	GSI1PK    string    `dynamodbav:"GSI1PK"`        // STATUS#{statusId}
	GSI1SK    string    `dynamodbav:"GSI1SK"`        // POLL
	TTL       int64     `dynamodbav:"TTL,omitempty"` // For poll expiration
	Poll      *Poll     `dynamodbav:"Poll"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt time.Time `dynamodbav:"UpdatedAt"`
}

// VoteRecord represents a vote record in DynamoDB
type VoteRecord struct {
	PK      string    `dynamodbav:"PK"` // POLL#{pollId}
	SK      string    `dynamodbav:"SK"` // VOTE#{voterId}
	VoterID string    `dynamodbav:"voterId"`
	Choices []int     `dynamodbav:"choices"`
	VotedAt time.Time `dynamodbav:"votedAt"`
}

// CreatePoll creates a new poll in DynamoDB
func (s *dynamoDBStorage) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	log := common.WithContext(ctx)

	// Validate poll
	if len(poll.Options) < 2 || len(poll.Options) > 4 {
		return fmt.Errorf("poll must have between 2 and 4 options")
	}

	// Generate poll ID if not provided
	if poll.ID == "" {
		poll.ID = fmt.Sprintf("%d-%s", time.Now().Unix(), generateRandomString(8))
	}

	// Set timestamps
	now := time.Now()
	poll.CreatedAt = now
	poll.UpdatedAt = now

	// Initialize vote tracking
	poll.VotesCount = 0
	poll.VotersCount = 0
	poll.Votes = make(map[string][]int)

	// Convert to internal poll type
	internalPoll := &Poll{
		ID:          poll.ID,
		StatusID:    poll.StatusID,
		CreatedBy:   poll.CreatedBy,
		Options:     poll.Options,
		Multiple:    poll.Multiple,
		HideTotals:  poll.HideTotals,
		ExpiresAt:   poll.ExpiresAt,
		CreatedAt:   poll.CreatedAt,
		UpdatedAt:   poll.UpdatedAt,
		VotesCount:  poll.VotesCount,
		VotersCount: poll.VotersCount,
		Votes:       poll.Votes,
	}

	// Create poll record
	record := &PollRecord{
		PK:        fmt.Sprintf("POLL#%s", poll.ID),
		SK:        "METADATA",
		GSI1PK:    fmt.Sprintf("STATUS#%s", poll.StatusID),
		GSI1SK:    "POLL",
		Poll:      internalPoll,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Set TTL for poll expiration (add 1 day buffer)
	if !poll.ExpiresAt.IsZero() {
		record.TTL = poll.ExpiresAt.Add(24 * time.Hour).Unix()
	}

	av, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal poll: %w", err)
	}

	// Put with condition that the item doesn't already exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if err != nil {
		log.Error("failed to create poll",
			zap.String("poll_id", poll.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create poll: %w", err)
	}

	log.Info("poll created successfully",
		zap.String("poll_id", poll.ID),
		zap.String("status_id", poll.StatusID),
		zap.Int("options_count", len(poll.Options)))

	return nil
}

// GetPoll retrieves a poll by ID
func (s *dynamoDBStorage) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	log := common.WithContext(ctx)

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("POLL#%s", pollID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		log.Error("failed to get poll",
			zap.String("poll_id", pollID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get poll: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("poll not found: %s", pollID)
	}

	var record PollRecord
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal poll: %w", err)
	}

	// Convert to storage poll
	poll := &storage.Poll{
		ID:          record.Poll.ID,
		StatusID:    record.Poll.StatusID,
		CreatedBy:   record.Poll.CreatedBy,
		Options:     record.Poll.Options,
		Multiple:    record.Poll.Multiple,
		HideTotals:  record.Poll.HideTotals,
		ExpiresAt:   record.Poll.ExpiresAt,
		CreatedAt:   record.Poll.CreatedAt,
		UpdatedAt:   record.Poll.UpdatedAt,
		VotesCount:  record.Poll.VotesCount,
		VotersCount: record.Poll.VotersCount,
		Votes:       record.Poll.Votes,
	}

	return poll, nil
}

// GetPollByStatusID retrieves a poll by its associated status ID
func (s *dynamoDBStorage) GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error) {
	log := common.WithContext(ctx)

	// Query GSI1 to find poll by status ID
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", statusID)},
			":sk": &types.AttributeValueMemberS{Value: "POLL"},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		log.Error("failed to query poll by status ID",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query poll: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("poll not found for status: %s", statusID)
	}

	var record PollRecord
	if err := attributevalue.UnmarshalMap(result.Items[0], &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal poll: %w", err)
	}

	// Convert to storage poll
	poll := &storage.Poll{
		ID:          record.Poll.ID,
		StatusID:    record.Poll.StatusID,
		CreatedBy:   record.Poll.CreatedBy,
		Options:     record.Poll.Options,
		Multiple:    record.Poll.Multiple,
		HideTotals:  record.Poll.HideTotals,
		ExpiresAt:   record.Poll.ExpiresAt,
		CreatedAt:   record.Poll.CreatedAt,
		UpdatedAt:   record.Poll.UpdatedAt,
		VotesCount:  record.Poll.VotesCount,
		VotersCount: record.Poll.VotersCount,
		Votes:       record.Poll.Votes,
	}

	return poll, nil
}

// VoteOnPoll records a vote on a poll
func (s *dynamoDBStorage) VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error {
	log := common.WithContext(ctx)

	// Get the poll first
	poll, err := s.GetPoll(ctx, pollID)
	if err != nil {
		return fmt.Errorf("failed to get poll: %w", err)
	}

	// Check if poll has expired
	if !poll.ExpiresAt.IsZero() && time.Now().After(poll.ExpiresAt) {
		return fmt.Errorf("poll has expired")
	}

	// Validate choices
	for _, choice := range choices {
		if choice < 0 || choice >= len(poll.Options) {
			return fmt.Errorf("invalid choice index: %d", choice)
		}
	}

	// Check multiple choice constraint
	if !poll.Multiple && len(choices) > 1 {
		return fmt.Errorf("poll does not allow multiple choices")
	}

	// Check if user already voted
	if _, hasVoted := poll.Votes[voterID]; hasVoted {
		return fmt.Errorf("user has already voted on this poll")
	}

	// Create vote record
	voteRecord := &VoteRecord{
		PK:      fmt.Sprintf("POLL#%s", pollID),
		SK:      fmt.Sprintf("VOTE#%s", voterID),
		VoterID: voterID,
		Choices: choices,
		VotedAt: time.Now(),
	}

	av, err := attributevalue.MarshalMap(voteRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal vote: %w", err)
	}

	// Put vote with condition that it doesn't already exist
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if err != nil {
		var cfe *types.ConditionalCheckFailedException
		if errors.As(err, &cfe) {
			return fmt.Errorf("user has already voted on this poll")
		}
		return fmt.Errorf("failed to record vote: %w", err)
	}

	// Update poll vote counts
	poll.Votes[voterID] = choices
	poll.VotersCount++
	poll.VotesCount += len(choices)
	poll.UpdatedAt = time.Now()

	// Update poll record
	if err := s.updatePollCounts(ctx, poll); err != nil {
		log.Error("failed to update poll counts",
			zap.String("poll_id", pollID),
			zap.Error(err))
		// Don't fail the vote, counts can be recalculated
	}

	log.Info("vote recorded successfully",
		zap.String("poll_id", pollID),
		zap.String("voter_id", voterID),
		zap.Ints("choices", choices))

	return nil
}

// GetPollVotes retrieves all votes for a poll
func (s *dynamoDBStorage) GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error) {
	// Query all votes for the poll
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("POLL#%s", pollID)},
			":sk": &types.AttributeValueMemberS{Value: "VOTE#"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query votes: %w", err)
	}

	votes := make(map[string][]int)
	for _, item := range result.Items {
		var voteRecord VoteRecord
		if err := attributevalue.UnmarshalMap(item, &voteRecord); err != nil {
			continue
		}
		votes[voteRecord.VoterID] = voteRecord.Choices
	}

	return votes, nil
}

// HasUserVoted checks if a user has voted on a poll
func (s *dynamoDBStorage) HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("POLL#%s", pollID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOTE#%s", userID)},
		},
	})
	if err != nil {
		return false, nil, fmt.Errorf("failed to check vote: %w", err)
	}

	if result.Item == nil {
		return false, nil, nil
	}

	var voteRecord VoteRecord
	if err := attributevalue.UnmarshalMap(result.Item, &voteRecord); err != nil {
		return false, nil, fmt.Errorf("failed to unmarshal vote: %w", err)
	}

	return true, voteRecord.Choices, nil
}

// updatePollCounts updates the vote counts on a poll
func (s *dynamoDBStorage) updatePollCounts(ctx context.Context, poll *storage.Poll) error {
	// Convert to internal poll type
	internalPoll := &Poll{
		ID:          poll.ID,
		StatusID:    poll.StatusID,
		CreatedBy:   poll.CreatedBy,
		Options:     poll.Options,
		Multiple:    poll.Multiple,
		HideTotals:  poll.HideTotals,
		ExpiresAt:   poll.ExpiresAt,
		CreatedAt:   poll.CreatedAt,
		UpdatedAt:   poll.UpdatedAt,
		VotesCount:  poll.VotesCount,
		VotersCount: poll.VotersCount,
		Votes:       poll.Votes,
	}

	// Update poll record
	record := &PollRecord{
		PK:        fmt.Sprintf("POLL#%s", poll.ID),
		SK:        "METADATA",
		GSI1PK:    fmt.Sprintf("STATUS#%s", poll.StatusID),
		GSI1SK:    "POLL",
		Poll:      internalPoll,
		UpdatedAt: poll.UpdatedAt,
	}

	// Set TTL if applicable
	if !poll.ExpiresAt.IsZero() {
		record.TTL = poll.ExpiresAt.Add(24 * time.Hour).Unix()
	}

	av, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal poll: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	return err
}
