package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// RemoteActorRecord represents a cached remote actor in DynamoDB
type RemoteActorRecord struct {
	PK        string `dynamodbav:"PK"`
	SK        string `dynamodbav:"SK"`
	Actor     *activitypub.Actor
	Handle    string    `dynamodbav:"Handle"`
	Domain    string    `dynamodbav:"Domain"`
	ExpiresAt time.Time `dynamodbav:"ExpiresAt"`
	CachedAt  time.Time `dynamodbav:"CachedAt"`
	UpdatedAt time.Time `dynamodbav:"UpdatedAt"`
	TTL       int64     `dynamodbav:"ttl"` // Unix timestamp for DynamoDB TTL
}

// CacheRemoteActor caches a remote actor with a TTL
func (s *dynamoDBStorage) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	log := common.WithContext(ctx)

	now := time.Now()
	expiresAt := now.Add(ttl)

	record := RemoteActorRecord{
		PK:        fmt.Sprintf("REMOTE_ACTOR#%s", handle),
		SK:        "PROFILE",
		Actor:     actor,
		Handle:    handle,
		Domain:    extractDomainFromHandle(handle),
		CachedAt:  now,
		UpdatedAt: now,
		ExpiresAt: expiresAt,
		TTL:       expiresAt.Unix(), // DynamoDB TTL
	}

	item, err := s.MarshalItem(record)
	if err != nil {
		return fmt.Errorf("failed to marshal remote actor: %w", err)
	}

	// Marshal the Actor separately to avoid issues with nested structs
	actorItem, err := s.MarshalItem(actor)
	if err != nil {
		return fmt.Errorf("failed to marshal actor data: %w", err)
	}
	item["Actor"] = &types.AttributeValueMemberM{Value: actorItem}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to cache remote actor: %w", err)
	}

	log.Debug("cached remote actor",
		zap.String("handle", handle),
		zap.String("actor_id", actor.ID),
		zap.Duration("ttl", ttl))

	return nil
}

// GetCachedRemoteActor retrieves a cached remote actor
func (s *dynamoDBStorage) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	log := common.WithContext(ctx)

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REMOTE_ACTOR#%s", handle)},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get cached remote actor: %w", err)
	}

	if result.Item == nil {
		// Extract username from handle for error
		username := strings.Split(handle, "@")[0]
		return nil, common.ActorNotFoundError{Username: username}
	}

	var record RemoteActorRecord
	err = s.UnmarshalItem(result.Item, &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal remote actor: %w", err)
	}

	// Check if the cache has expired
	if time.Now().After(record.ExpiresAt) {
		log.Debug("cached remote actor expired",
			zap.String("handle", handle),
			zap.Time("expired_at", record.ExpiresAt))
		// Extract username from handle for error
		username := strings.Split(handle, "@")[0]
		return nil, common.ActorNotFoundError{Username: username}
	}

	log.Debug("retrieved cached remote actor",
		zap.String("handle", handle),
		zap.String("actor_id", record.Actor.ID))

	return record.Actor, nil
}

// RefreshRemoteActorCache updates the cache timestamp for a remote actor
func (s *dynamoDBStorage) RefreshRemoteActorCache(ctx context.Context, handle string, ttl time.Duration) error {
	now := time.Now()
	expiresAt := now.Add(ttl)

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("REMOTE_ACTOR#%s", handle)},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression: aws.String("SET UpdatedAt = :now, ExpiresAt = :expires, #ttl = :ttl"),
		ExpressionAttributeNames: map[string]string{
			"#ttl": "ttl", // ttl is a reserved word in DynamoDB
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now":     &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":expires": &types.AttributeValueMemberS{Value: expiresAt.Format(time.RFC3339)},
			":ttl":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", expiresAt.Unix())},
		},
	})

	return err
}

// ListCachedRemoteActors returns all cached remote actors (for debugging/admin)
func (s *dynamoDBStorage) ListCachedRemoteActors(ctx context.Context, limit int32) ([]*RemoteActorRecord, error) {
	var actors []*RemoteActorRecord

	// Query all items with PK starting with REMOTE_ACTOR#
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("begins_with(PK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: "REMOTE_ACTOR#"},
		},
		Limit: &limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list cached remote actors: %w", err)
	}

	for _, item := range result.Items {
		var record RemoteActorRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			continue // Skip invalid records
		}

		// Skip expired records
		if time.Now().After(record.ExpiresAt) {
			continue
		}

		actors = append(actors, &record)
	}

	return actors, nil
}

// extractDomainFromHandle extracts the domain from a handle like user@domain
func extractDomainFromHandle(handle string) string {
	parts := strings.Split(handle, "@")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

// CleanupExpiredRemoteActors removes expired remote actor cache entries
// This is handled automatically by DynamoDB TTL, but this method can be used for manual cleanup
func (s *dynamoDBStorage) CleanupExpiredRemoteActors(ctx context.Context) (int, error) {
	now := time.Now()
	var deletedCount int

	// Query all remote actors
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("begins_with(PK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: "REMOTE_ACTOR#"},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to query remote actors: %w", err)
	}

	// Delete expired entries
	for _, item := range result.Items {
		var record RemoteActorRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			continue
		}

		if now.After(record.ExpiresAt) {
			_, err = s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: s.getTableName(),
				Key: map[string]types.AttributeValue{
					"PK": item["PK"],
					"SK": item["SK"],
				},
			})
			if err == nil {
				deletedCount++
			}
		}
	}

	return deletedCount, nil
}
