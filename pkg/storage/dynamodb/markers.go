package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// MarkerRecord represents a timeline marker in DynamoDB
type MarkerRecord struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	LastReadID string    `dynamodbav:"LastReadID"`
	UpdatedAt  time.Time `dynamodbav:"UpdatedAt"`
	Version    int       `dynamodbav:"Version"`
}

// SaveMarker saves or updates a timeline position marker
func (s *dynamoDBStorage) SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error {
	pk := fmt.Sprintf("USER#%s", username)
	sk := fmt.Sprintf("MARKER#%s", timeline)

	// Get current marker to check version
	existingMarker, err := s.GetMarkers(ctx, username, []string{timeline})
	if err != nil {
		s.logger().Error("failed to get existing marker", zap.Error(err))
		// Continue anyway, might be first marker
	}

	// Check version conflict
	if existingMarker != nil && existingMarker[timeline] != nil {
		if existingMarker[timeline].Version >= version {
			// Don't update if the existing version is newer or same
			return nil
		}
	}

	// Create marker record
	record := MarkerRecord{
		PK:         pk,
		SK:         sk,
		LastReadID: lastReadID,
		UpdatedAt:  time.Now(),
		Version:    version,
	}

	// Marshal to DynamoDB attributes
	av, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal marker: %w", err)
	}

	// Put item
	input := &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to save marker: %w", err)
	}

	s.logger().Debug("saved marker",
		zap.String("username", username),
		zap.String("timeline", timeline),
		zap.String("last_read_id", lastReadID),
		zap.Int("version", version))

	return nil
}

// GetMarkers retrieves timeline position markers for specified timelines
func (s *dynamoDBStorage) GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	if len(timelines) == 0 {
		// Get all markers if no specific timelines requested
		timelines = []string{"home", "notifications"}
	}

	markers := make(map[string]*storage.Marker)
	pk := fmt.Sprintf("USER#%s", username)

	// Get each marker individually
	for _, timeline := range timelines {
		sk := fmt.Sprintf("MARKER#%s", timeline)

		input := &dynamodb.GetItemInput{
			TableName: s.getTableName(),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: pk},
				"SK": &types.AttributeValueMemberS{Value: sk},
			},
		}

		result, err := s.client.GetItem(ctx, input)
		if err != nil {
			s.logger().Warn("failed to get marker",
				zap.String("timeline", timeline),
				zap.Error(err))
			continue
		}

		if result.Item != nil {
			var record MarkerRecord
			if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
				s.logger().Warn("failed to unmarshal marker", zap.Error(err))
				continue
			}

			markers[timeline] = &storage.Marker{
				LastReadID: record.LastReadID,
				UpdatedAt:  record.UpdatedAt,
				Version:    record.Version,
			}
		}
	}

	return markers, nil
}
