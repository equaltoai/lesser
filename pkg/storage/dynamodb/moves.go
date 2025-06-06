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
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// MoveRecord represents a move activity stored in DynamoDB
type MoveRecord struct {
	PK        string        `dynamodbav:"PK"`     // MOVE#ACTOR#{actor}
	SK        string        `dynamodbav:"SK"`     // TARGET#{target}
	GSI1PK    string        `dynamodbav:"GSI1PK"` // MOVE#TARGET#{target}
	GSI1SK    string        `dynamodbav:"GSI1SK"` // ACTOR#{actor}
	Move      *storage.Move `dynamodbav:"Move"`
	CreatedAt time.Time     `dynamodbav:"CreatedAt"`
	TTL       *int64        `dynamodbav:"TTL,omitempty"` // Optional TTL for cleanup
}

// CreateMove creates a new move activity in the database
func (s *dynamoDBStorage) CreateMove(ctx context.Context, move *storage.Move) error {
	log := common.WithContext(ctx).With(
		zap.String("move_id", move.ID),
		zap.String("actor", move.Actor),
		zap.String("target", move.Target),
	)

	move.CreatedAt = time.Now()

	// Create the move record
	record := &MoveRecord{
		PK:        fmt.Sprintf("MOVE#ACTOR#%s", move.Actor),
		SK:        fmt.Sprintf("TARGET#%s", move.Target),
		GSI1PK:    fmt.Sprintf("MOVE#TARGET#%s", move.Target),
		GSI1SK:    fmt.Sprintf("ACTOR#%s", move.Actor),
		Move:      move,
		CreatedAt: move.CreatedAt,
	}

	item, err := s.MarshalItem(record)
	if err != nil {
		log.Error("failed to marshal move record", zap.Error(err))
		return fmt.Errorf("failed to marshal move record: %w", err)
	}

	// Use a condition to prevent duplicate moves from the same actor
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.getTableName(),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			log.Warn("actor has already moved", zap.String("actor", move.Actor))
			return fmt.Errorf("actor %s has already moved", move.Actor)
		}
		log.Error("failed to create move", zap.Error(err))
		return fmt.Errorf("failed to create move: %w", err)
	}

	log.Info("move created successfully")
	return nil
}

// GetMove retrieves the most recent move for an actor
func (s *dynamoDBStorage) GetMove(ctx context.Context, actor string) (*storage.Move, error) {
	log := common.WithContext(ctx).With(zap.String("actor", actor))

	// Query for moves from this actor
	keyCondition := expression.Key("PK").Equal(expression.Value(fmt.Sprintf("MOVE#ACTOR#%s", actor)))

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		log.Error("failed to build expression", zap.Error(err))
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 s.getTableName(),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(1),    // Get the most recent move
		ScanIndexForward:          aws.Bool(false), // Sort descending
	})
	if err != nil {
		log.Error("failed to query move", zap.Error(err))
		return nil, fmt.Errorf("failed to query move: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no move found for actor: %s", actor)
	}

	var record MoveRecord
	if err := attributevalue.UnmarshalMap(result.Items[0], &record); err != nil {
		log.Error("failed to unmarshal move record", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal move record: %w", err)
	}

	return record.Move, nil
}

// GetMoveByTarget retrieves all moves to a specific target account
func (s *dynamoDBStorage) GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error) {
	log := common.WithContext(ctx).With(zap.String("target", target))

	// Query using GSI1 for moves to this target
	keyCondition := expression.Key("GSI1PK").Equal(expression.Value(fmt.Sprintf("MOVE#TARGET#%s", target)))

	expr, err := expression.NewBuilder().
		WithKeyCondition(keyCondition).
		Build()
	if err != nil {
		log.Error("failed to build expression", zap.Error(err))
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 s.getTableName(),
		IndexName:                 aws.String("GSI1"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		log.Error("failed to query moves by target", zap.Error(err))
		return nil, fmt.Errorf("failed to query moves by target: %w", err)
	}

	moves := make([]*storage.Move, 0, len(result.Items))
	for _, item := range result.Items {
		var record MoveRecord
		if err := attributevalue.UnmarshalMap(item, &record); err != nil {
			log.Error("failed to unmarshal move record", zap.Error(err))
			continue
		}
		moves = append(moves, record.Move)
	}

	log.Info("retrieved moves by target", zap.Int("count", len(moves)))
	return moves, nil
}

// HasMovedFrom checks if newActor has moved from oldActor
func (s *dynamoDBStorage) HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error) {
	log := common.WithContext(ctx).With(
		zap.String("old_actor", oldActor),
		zap.String("new_actor", newActor),
	)

	// Check if there's a move from oldActor to newActor
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MOVE#ACTOR#%s", oldActor)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TARGET#%s", newActor)},
		},
	})
	if err != nil {
		log.Error("failed to check move", zap.Error(err))
		return false, fmt.Errorf("failed to check move: %w", err)
	}

	exists := result.Item != nil
	log.Info("checked move relationship", zap.Bool("exists", exists))
	return exists, nil
}
