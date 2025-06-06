package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/storage"
)

// CustomEmojiRecord represents a custom emoji in DynamoDB
type CustomEmojiRecord struct {
	PK                  string    `dynamodbav:"PK"` // EMOJI#shortcode
	SK                  string    `dynamodbav:"SK"` // EMOJI
	Shortcode           string    `dynamodbav:"Shortcode"`
	URL                 string    `dynamodbav:"URL"`
	StaticURL           string    `dynamodbav:"StaticURL"`
	VisibleInPicker     bool      `dynamodbav:"VisibleInPicker"`
	Category            string    `dynamodbav:"Category,omitempty"`
	CreatedAt           time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt           time.Time `dynamodbav:"UpdatedAt"`
	Disabled            bool      `dynamodbav:"Disabled"`
	Domain              string    `dynamodbav:"Domain,omitempty"`
	ImageRemoteURL      string    `dynamodbav:"ImageRemoteURL,omitempty"`
	ImageStorageVersion int       `dynamodbav:"ImageStorageVersion"`
	ImageFileSize       int64     `dynamodbav:"ImageFileSize"`
	ImageContentType    string    `dynamodbav:"ImageContentType"`
	ImageWidth          int       `dynamodbav:"ImageWidth"`
	ImageHeight         int       `dynamodbav:"ImageHeight"`
	ImageUpdatedAt      time.Time `dynamodbav:"ImageUpdatedAt"`
}

// CreateCustomEmoji creates a new custom emoji
func (s *dynamoDBStorage) CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	now := time.Now()
	emoji.CreatedAt = now
	emoji.UpdatedAt = now
	emoji.ImageUpdatedAt = now

	record := CustomEmojiRecord{
		PK:                  fmt.Sprintf("EMOJI#%s", emoji.Shortcode),
		SK:                  "EMOJI",
		Shortcode:           emoji.Shortcode,
		URL:                 emoji.URL,
		StaticURL:           emoji.StaticURL,
		VisibleInPicker:     emoji.VisibleInPicker,
		Category:            emoji.Category,
		CreatedAt:           emoji.CreatedAt,
		UpdatedAt:           emoji.UpdatedAt,
		Disabled:            emoji.Disabled,
		Domain:              emoji.Domain,
		ImageRemoteURL:      emoji.ImageRemoteURL,
		ImageStorageVersion: emoji.ImageStorageVersion,
		ImageFileSize:       emoji.ImageFileSize,
		ImageContentType:    emoji.ImageContentType,
		ImageWidth:          emoji.ImageWidth,
		ImageHeight:         emoji.ImageHeight,
		ImageUpdatedAt:      emoji.ImageUpdatedAt,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal custom emoji: %w", err)
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
		return fmt.Errorf("failed to create custom emoji: %w", err)
	}

	return nil
}

// GetCustomEmoji retrieves a custom emoji by shortcode
func (s *dynamoDBStorage) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EMOJI#%s", shortcode)},
			"SK": &types.AttributeValueMemberS{Value: "EMOJI"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom emoji: %w", err)
	}

	if result.Item == nil {
		return nil, storage.ErrNotFound
	}

	var record CustomEmojiRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal custom emoji: %w", err)
	}

	return s.recordToCustomEmoji(&record), nil
}

// GetCustomEmojis retrieves all custom emojis
func (s *dynamoDBStorage) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	// Use a scan with filter for emoji items
	// In production, you might want to use a GSI for better performance
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND SK = :sk AND (#disabled = :disabled OR attribute_not_exists(#disabled))"),
		ExpressionAttributeNames: map[string]string{
			"#disabled": "Disabled",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix":   &types.AttributeValueMemberS{Value: "EMOJI#"},
			":sk":       &types.AttributeValueMemberS{Value: "EMOJI"},
			":disabled": &types.AttributeValueMemberBOOL{Value: false},
		},
	}

	emojis := make([]*storage.CustomEmoji, 0)
	paginator := dynamodb.NewScanPaginator(s.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan custom emojis: %w", err)
		}

		for _, item := range page.Items {
			var record CustomEmojiRecord
			if err := s.UnmarshalItem(item, &record); err != nil {
				s.logger().Warn("failed to unmarshal custom emoji", zap.Error(err))
				continue
			}

			// Skip disabled emojis unless they're remote emojis
			if record.Disabled && record.Domain == "" {
				continue
			}

			emojis = append(emojis, s.recordToCustomEmoji(&record))
		}
	}

	return emojis, nil
}

// UpdateCustomEmoji updates an existing custom emoji
func (s *dynamoDBStorage) UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	emoji.UpdatedAt = time.Now()

	record := CustomEmojiRecord{
		PK:                  fmt.Sprintf("EMOJI#%s", emoji.Shortcode),
		SK:                  "EMOJI",
		Shortcode:           emoji.Shortcode,
		URL:                 emoji.URL,
		StaticURL:           emoji.StaticURL,
		VisibleInPicker:     emoji.VisibleInPicker,
		Category:            emoji.Category,
		CreatedAt:           emoji.CreatedAt,
		UpdatedAt:           emoji.UpdatedAt,
		Disabled:            emoji.Disabled,
		Domain:              emoji.Domain,
		ImageRemoteURL:      emoji.ImageRemoteURL,
		ImageStorageVersion: emoji.ImageStorageVersion,
		ImageFileSize:       emoji.ImageFileSize,
		ImageContentType:    emoji.ImageContentType,
		ImageWidth:          emoji.ImageWidth,
		ImageHeight:         emoji.ImageHeight,
		ImageUpdatedAt:      emoji.ImageUpdatedAt,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal custom emoji: %w", err)
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
		return fmt.Errorf("failed to update custom emoji: %w", err)
	}

	return nil
}

// DeleteCustomEmoji deletes a custom emoji
func (s *dynamoDBStorage) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EMOJI#%s", shortcode)},
			"SK": &types.AttributeValueMemberS{Value: "EMOJI"},
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND attribute_exists(SK)"),
	}

	_, err := s.client.DeleteItem(ctx, input)
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to delete custom emoji: %w", err)
	}

	return nil
}

// GetCustomEmojisByCategory retrieves custom emojis by category
func (s *dynamoDBStorage) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	// Similar to GetCustomEmojis but with category filter
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :prefix) AND SK = :sk AND #category = :category AND (#disabled = :disabled OR attribute_not_exists(#disabled))"),
		ExpressionAttributeNames: map[string]string{
			"#category": "Category",
			"#disabled": "Disabled",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix":   &types.AttributeValueMemberS{Value: "EMOJI#"},
			":sk":       &types.AttributeValueMemberS{Value: "EMOJI"},
			":category": &types.AttributeValueMemberS{Value: category},
			":disabled": &types.AttributeValueMemberBOOL{Value: false},
		},
	}

	emojis := make([]*storage.CustomEmoji, 0)
	paginator := dynamodb.NewScanPaginator(s.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan custom emojis by category: %w", err)
		}

		for _, item := range page.Items {
			var record CustomEmojiRecord
			if err := s.UnmarshalItem(item, &record); err != nil {
				s.logger().Warn("failed to unmarshal custom emoji", zap.Error(err))
				continue
			}

			// Skip disabled emojis
			if record.Disabled && record.Domain == "" {
				continue
			}

			emojis = append(emojis, s.recordToCustomEmoji(&record))
		}
	}

	return emojis, nil
}

// Helper function to convert record to custom emoji
func (s *dynamoDBStorage) recordToCustomEmoji(record *CustomEmojiRecord) *storage.CustomEmoji {
	return &storage.CustomEmoji{
		Shortcode:           record.Shortcode,
		URL:                 record.URL,
		StaticURL:           record.StaticURL,
		VisibleInPicker:     record.VisibleInPicker,
		Category:            record.Category,
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
		Disabled:            record.Disabled,
		Domain:              record.Domain,
		ImageRemoteURL:      record.ImageRemoteURL,
		ImageStorageVersion: record.ImageStorageVersion,
		ImageFileSize:       record.ImageFileSize,
		ImageContentType:    record.ImageContentType,
		ImageWidth:          record.ImageWidth,
		ImageHeight:         record.ImageHeight,
		ImageUpdatedAt:      record.ImageUpdatedAt,
	}
}
