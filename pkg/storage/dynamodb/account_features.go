package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// Constants for DynamoDB key patterns
const (
	AccountPinPKPrefix  = "ACCOUNT_PIN#"
	AccountPinSK        = "PIN#"
	AccountNotePKPrefix = "ACCOUNT_NOTE#"
	AccountNoteSK       = "NOTE#"
)

// AccountPinRecord represents a pinned account in DynamoDB
type AccountPinRecord struct {
	PK        string              `dynamodbav:"PK"`
	SK        string              `dynamodbav:"SK"`
	Pin       *storage.AccountPin `dynamodbav:"Pin"`
	CreatedAt time.Time           `dynamodbav:"CreatedAt"`
}

// AccountNoteRecord represents an account note in DynamoDB
type AccountNoteRecord struct {
	PK        string               `dynamodbav:"PK"`
	SK        string               `dynamodbav:"SK"`
	Note      *storage.AccountNote `dynamodbav:"Note"`
	UpdatedAt time.Time            `dynamodbav:"UpdatedAt"`
}

// CreateAccountPin creates a new account pin (endorsed account)
func (s *dynamoDBStorage) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	if pin.CreatedAt.IsZero() {
		pin.CreatedAt = time.Now()
	}

	// Check if already pinned
	exists, err := s.IsAccountPinned(ctx, pin.Username, pin.PinnedActorID)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("account already pinned")
	}

	// Create the record
	record := AccountPinRecord{
		PK:        fmt.Sprintf("%s%s", AccountPinPKPrefix, pin.Username),
		SK:        fmt.Sprintf("%s%s", AccountPinSK, pin.PinnedActorID),
		Pin:       pin,
		CreatedAt: time.Now(),
	}

	// Marshal the record
	av, err := s.MarshalItem(record)
	if err != nil {
		s.logger().Error("failed to marshal account pin record", zap.Error(err))
		return err
	}

	// Put the item
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		s.logger().Error("failed to create account pin", zap.Error(err))
		return err
	}

	return nil
}

// DeleteAccountPin deletes an account pin
func (s *dynamoDBStorage) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountPinPKPrefix, username)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountPinSK, pinnedActorID)},
	}

	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		s.logger().Error("failed to delete account pin", zap.Error(err))
		return err
	}

	return nil
}

// GetAccountPins retrieves all pinned accounts for a user
func (s *dynamoDBStorage) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	// Query for all pins for this user
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountPinPKPrefix, username)},
			":sk": &types.AttributeValueMemberS{Value: AccountPinSK},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		s.logger().Error("failed to query account pins", zap.Error(err))
		return nil, err
	}

	// Unmarshal the results
	pins := make([]*storage.AccountPin, 0, len(result.Items))
	for _, item := range result.Items {
		var record AccountPinRecord
		if err := s.UnmarshalItem(item, &record); err != nil {
			s.logger().Error("failed to unmarshal account pin record", zap.Error(err))
			continue
		}
		pins = append(pins, record.Pin)
	}

	return pins, nil
}

// IsAccountPinned checks if an account is pinned
func (s *dynamoDBStorage) IsAccountPinned(ctx context.Context, username, pinnedActorID string) (bool, error) {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountPinPKPrefix, username)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountPinSK, pinnedActorID)},
	}

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		s.logger().Error("failed to check account pin", zap.Error(err))
		return false, err
	}

	return result.Item != nil, nil
}

// CreateAccountNote creates a new private note on an account
func (s *dynamoDBStorage) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now()
	}
	if note.UpdatedAt.IsZero() {
		note.UpdatedAt = note.CreatedAt
	}

	// Create the record
	record := AccountNoteRecord{
		PK:        fmt.Sprintf("%s%s", AccountNotePKPrefix, note.Username),
		SK:        fmt.Sprintf("%s%s", AccountNoteSK, note.TargetActorID),
		Note:      note,
		UpdatedAt: time.Now(),
	}

	// Marshal the record
	av, err := s.MarshalItem(record)
	if err != nil {
		s.logger().Error("failed to marshal account note record", zap.Error(err))
		return err
	}

	// Put the item
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		s.logger().Error("failed to create account note", zap.Error(err))
		return err
	}

	return nil
}

// UpdateAccountNote updates an existing private note on an account
func (s *dynamoDBStorage) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	note.UpdatedAt = time.Now()

	// Create the record
	record := AccountNoteRecord{
		PK:        fmt.Sprintf("%s%s", AccountNotePKPrefix, note.Username),
		SK:        fmt.Sprintf("%s%s", AccountNoteSK, note.TargetActorID),
		Note:      note,
		UpdatedAt: time.Now(),
	}

	// Marshal the record
	av, err := s.MarshalItem(record)
	if err != nil {
		s.logger().Error("failed to marshal account note record", zap.Error(err))
		return err
	}

	// Put the item (overwrites existing)
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		s.logger().Error("failed to update account note", zap.Error(err))
		return err
	}

	return nil
}

// GetAccountNote retrieves a private note on an account
func (s *dynamoDBStorage) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountNotePKPrefix, username)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountNoteSK, targetActorID)},
	}

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		s.logger().Error("failed to get account note", zap.Error(err))
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
	}

	// Unmarshal the record
	var record AccountNoteRecord
	if err := s.UnmarshalItem(result.Item, &record); err != nil {
		s.logger().Error("failed to unmarshal account note record", zap.Error(err))
		return nil, err
	}

	return record.Note, nil
}

// DeleteAccountNote deletes a private note on an account
func (s *dynamoDBStorage) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountNotePKPrefix, username)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("%s%s", AccountNoteSK, targetActorID)},
	}

	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		s.logger().Error("failed to delete account note", zap.Error(err))
		return err
	}

	return nil
}
