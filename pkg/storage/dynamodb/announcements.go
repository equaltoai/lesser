package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
)

// AnnouncementRecord represents an announcement in DynamoDB
type AnnouncementRecord struct {
	PK          string                `dynamodbav:"PK"` // ANNOUNCEMENT#id
	SK          string                `dynamodbav:"SK"` // ANNOUNCEMENT
	ID          string                `dynamodbav:"ID"`
	Content     string                `dynamodbav:"Content"`             // HTML content
	Text        string                `dynamodbav:"Text"`                // Plain text version
	PublishedAt time.Time             `dynamodbav:"PublishedAt"`         // When it was published
	UpdatedAt   time.Time             `dynamodbav:"UpdatedAt"`           // When it was last updated
	AllDay      bool                  `dynamodbav:"AllDay"`              // Whether it's an all-day announcement
	StartsAt    *time.Time            `dynamodbav:"StartsAt,omitempty"`  // When the announcement starts
	EndsAt      *time.Time            `dynamodbav:"EndsAt,omitempty"`    // When the announcement ends
	Reactions   []storage.Reaction    `dynamodbav:"Reactions,omitempty"` // Available reactions
	Tags        []string              `dynamodbav:"Tags,omitempty"`      // Hashtags
	Emojis      []storage.CustomEmoji `dynamodbav:"Emojis,omitempty"`    // Custom emojis
	Mentions    []storage.Mention     `dynamodbav:"Mentions,omitempty"`  // Mentions
	CreatedBy   string                `dynamodbav:"CreatedBy"`           // Admin who created it
	CreatedAt   time.Time             `dynamodbav:"CreatedAt"`
}

// AnnouncementDismissalRecord represents a dismissal in DynamoDB
type AnnouncementDismissalRecord struct {
	PK             string    `dynamodbav:"PK"` // USER#username
	SK             string    `dynamodbav:"SK"` // ANNOUNCEMENT_DISMISSED#announcement_id
	Username       string    `dynamodbav:"Username"`
	AnnouncementID string    `dynamodbav:"AnnouncementID"`
	DismissedAt    time.Time `dynamodbav:"DismissedAt"`
}

// AnnouncementReactionRecord represents a reaction in DynamoDB
type AnnouncementReactionRecord struct {
	PK             string    `dynamodbav:"PK"` // ANNOUNCEMENT_REACTION#announcement_id
	SK             string    `dynamodbav:"SK"` // USER#username#emoji_name
	Username       string    `dynamodbav:"Username"`
	AnnouncementID string    `dynamodbav:"AnnouncementID"`
	EmojiName      string    `dynamodbav:"EmojiName"`
	ReactedAt      time.Time `dynamodbav:"ReactedAt"`
}

// CreateAnnouncement creates a new announcement
func (s *dynamoDBStorage) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	if announcement.ID == "" {
		announcement.ID = uuid.New().String()
	}

	now := time.Now()
	announcement.PublishedAt = now
	announcement.UpdatedAt = now

	record := AnnouncementRecord{
		PK:          fmt.Sprintf("ANNOUNCEMENT#%s", announcement.ID),
		SK:          "ANNOUNCEMENT",
		ID:          announcement.ID,
		Content:     announcement.Content,
		Text:        announcement.Text,
		PublishedAt: announcement.PublishedAt,
		UpdatedAt:   announcement.UpdatedAt,
		AllDay:      announcement.AllDay,
		StartsAt:    announcement.StartsAt,
		EndsAt:      announcement.EndsAt,
		Reactions:   announcement.Reactions,
		Tags:        announcement.Tags,
		Emojis:      announcement.Emojis,
		Mentions:    announcement.Mentions,
		CreatedBy:   announcement.CreatedBy,
		CreatedAt:   now,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal announcement: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return storage.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create announcement: %w", err)
	}

	return nil
}

// GetAnnouncement retrieves a single announcement by ID
func (s *dynamoDBStorage) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANNOUNCEMENT#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "ANNOUNCEMENT"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get announcement: %w", err)
	}

	if result.Item == nil {
		return nil, storage.ErrNotFound
	}

	var record AnnouncementRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal announcement: %w", err)
	}

	return s.recordToAnnouncement(&record), nil
}

// GetAnnouncements retrieves all announcements (active or all)
func (s *dynamoDBStorage) GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error) {
	// In a real implementation, we might want to use a GSI for efficient querying
	// For now, we'll scan with a filter
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND SK = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: "ANNOUNCEMENT#"},
			":sk":     &types.AttributeValueMemberS{Value: "ANNOUNCEMENT"},
		},
	}

	announcements := make([]*storage.Announcement, 0)
	paginator := dynamodb.NewScanPaginator(s.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan announcements: %w", err)
		}

		for _, item := range page.Items {
			var record AnnouncementRecord
			if err := s.UnmarshalItem(item, &record); err != nil {
				s.logger().Warn("failed to unmarshal announcement", zap.Error(err))
				continue
			}

			announcement := s.recordToAnnouncement(&record)

			// Filter active announcements if requested
			if active {
				now := time.Now()
				// Skip if not yet started
				if announcement.StartsAt != nil && announcement.StartsAt.After(now) {
					continue
				}
				// Skip if already ended
				if announcement.EndsAt != nil && announcement.EndsAt.Before(now) {
					continue
				}
			}

			announcements = append(announcements, announcement)
		}
	}

	return announcements, nil
}

// UpdateAnnouncement updates an existing announcement
func (s *dynamoDBStorage) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	announcement.UpdatedAt = time.Now()

	record := AnnouncementRecord{
		PK:          fmt.Sprintf("ANNOUNCEMENT#%s", announcement.ID),
		SK:          "ANNOUNCEMENT",
		ID:          announcement.ID,
		Content:     announcement.Content,
		Text:        announcement.Text,
		PublishedAt: announcement.PublishedAt,
		UpdatedAt:   announcement.UpdatedAt,
		AllDay:      announcement.AllDay,
		StartsAt:    announcement.StartsAt,
		EndsAt:      announcement.EndsAt,
		Reactions:   announcement.Reactions,
		Tags:        announcement.Tags,
		Emojis:      announcement.Emojis,
		Mentions:    announcement.Mentions,
		CreatedBy:   announcement.CreatedBy,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal announcement: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to update announcement: %w", err)
	}

	return nil
}

// DeleteAnnouncement deletes an announcement
func (s *dynamoDBStorage) DeleteAnnouncement(ctx context.Context, id string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANNOUNCEMENT#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "ANNOUNCEMENT"},
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to delete announcement: %w", err)
	}

	// Clean up related dismissals and reactions
	// Note: These are best-effort cleanups - we don't fail the deletion if cleanup fails

	// Clean up reactions
	reactionInput := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANNOUNCEMENT_REACTION#%s", id)},
		},
	}

	reactionPaginator := dynamodb.NewQueryPaginator(s.client, reactionInput)
	for reactionPaginator.HasMorePages() {
		page, err := reactionPaginator.NextPage(ctx)
		if err != nil {
			s.logger().Warn("failed to query reactions for cleanup",
				zap.String("announcement_id", id),
				zap.Error(err))
			break
		}

		// Delete each reaction
		for _, item := range page.Items {
			pk := item["PK"]
			sk := item["SK"]
			if pk != nil && sk != nil {
				_, deleteErr := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
					TableName: aws.String(s.tableName),
					Key: map[string]types.AttributeValue{
						"PK": pk,
						"SK": sk,
					},
				})
				if deleteErr != nil {
					s.logger().Warn("failed to delete reaction during cleanup",
						zap.String("announcement_id", id),
						zap.Error(deleteErr))
				}
			}
		}
	}

	// Clean up dismissals
	// Since dismissals are stored under user keys, we need to scan for them
	dismissalInput := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("ends_with(SK, :suffix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":suffix": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANNOUNCEMENT_DISMISSED#%s", id)},
		},
	}

	dismissalPaginator := dynamodb.NewScanPaginator(s.client, dismissalInput)
	for dismissalPaginator.HasMorePages() {
		page, err := dismissalPaginator.NextPage(ctx)
		if err != nil {
			s.logger().Warn("failed to scan dismissals for cleanup",
				zap.String("announcement_id", id),
				zap.Error(err))
			break
		}

		// Delete each dismissal
		for _, item := range page.Items {
			pk := item["PK"]
			sk := item["SK"]
			if pk != nil && sk != nil {
				_, deleteErr := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
					TableName: aws.String(s.tableName),
					Key: map[string]types.AttributeValue{
						"PK": pk,
						"SK": sk,
					},
				})
				if deleteErr != nil {
					s.logger().Warn("failed to delete dismissal during cleanup",
						zap.String("announcement_id", id),
						zap.Error(deleteErr))
				}
			}
		}
	}

	return nil
}

// DismissAnnouncement marks an announcement as dismissed by a user
func (s *dynamoDBStorage) DismissAnnouncement(ctx context.Context, username, announcementID string) error {
	record := AnnouncementDismissalRecord{
		PK:             s.userPK(username),
		SK:             fmt.Sprintf("ANNOUNCEMENT_DISMISSED#%s", announcementID),
		Username:       username,
		AnnouncementID: announcementID,
		DismissedAt:    time.Now(),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal dismissal: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to dismiss announcement: %w", err)
	}

	return nil
}

// IsDismissed checks if a user has dismissed an announcement
func (s *dynamoDBStorage) IsDismissed(ctx context.Context, username, announcementID string) (bool, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: s.userPK(username)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANNOUNCEMENT_DISMISSED#%s", announcementID)},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check dismissal: %w", err)
	}

	return result.Item != nil, nil
}

// GetDismissedAnnouncements gets all announcement IDs dismissed by a user
func (s *dynamoDBStorage) GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: s.userPK(username)},
			":prefix": &types.AttributeValueMemberS{Value: "ANNOUNCEMENT_DISMISSED#"},
		},
	}

	announcementIDs := make([]string, 0)
	paginator := dynamodb.NewQueryPaginator(s.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query dismissed announcements: %w", err)
		}

		for _, item := range page.Items {
			var record AnnouncementDismissalRecord
			if err := s.UnmarshalItem(item, &record); err != nil {
				s.logger().Warn("failed to unmarshal dismissal", zap.Error(err))
				continue
			}
			announcementIDs = append(announcementIDs, record.AnnouncementID)
		}
	}

	return announcementIDs, nil
}

// AddAnnouncementReaction adds a user's reaction to an announcement
func (s *dynamoDBStorage) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	record := AnnouncementReactionRecord{
		PK:             fmt.Sprintf("ANNOUNCEMENT_REACTION#%s", announcementID),
		SK:             fmt.Sprintf("USER#%s#%s", username, emojiName),
		Username:       username,
		AnnouncementID: announcementID,
		EmojiName:      emojiName,
		ReactedAt:      time.Now(),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal reaction: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	}

	_, err = s.client.PutItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			// Already reacted
			return nil
		}
		return fmt.Errorf("failed to add reaction: %w", err)
	}

	return nil
}

// RemoveAnnouncementReaction removes a user's reaction from an announcement
func (s *dynamoDBStorage) RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANNOUNCEMENT_REACTION#%s", announcementID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#%s", username, emojiName)},
		},
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to remove reaction: %w", err)
	}

	return nil
}

// GetAnnouncementReactions gets all reactions for an announcement
func (s *dynamoDBStorage) GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ANNOUNCEMENT_REACTION#%s", announcementID)},
		},
	}

	reactions := make(map[string][]string)
	paginator := dynamodb.NewQueryPaginator(s.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query reactions: %w", err)
		}

		for _, item := range page.Items {
			var record AnnouncementReactionRecord
			if err := s.UnmarshalItem(item, &record); err != nil {
				s.logger().Warn("failed to unmarshal reaction", zap.Error(err))
				continue
			}

			if _, exists := reactions[record.EmojiName]; !exists {
				reactions[record.EmojiName] = make([]string, 0)
			}
			reactions[record.EmojiName] = append(reactions[record.EmojiName], record.Username)
		}
	}

	return reactions, nil
}

// Helper function to convert record to announcement
func (s *dynamoDBStorage) recordToAnnouncement(record *AnnouncementRecord) *storage.Announcement {
	return &storage.Announcement{
		ID:          record.ID,
		Content:     record.Content,
		Text:        record.Text,
		PublishedAt: record.PublishedAt,
		UpdatedAt:   record.UpdatedAt,
		AllDay:      record.AllDay,
		StartsAt:    record.StartsAt,
		EndsAt:      record.EndsAt,
		Reactions:   record.Reactions,
		Tags:        record.Tags,
		Emojis:      record.Emojis,
		Mentions:    record.Mentions,
		CreatedBy:   record.CreatedBy,
	}
}
