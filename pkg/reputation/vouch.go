package reputation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// VouchManager handles vouch creation and management
type VouchManager struct {
	db        *dynamodb.Client
	tableName string
	signer    *Signer
	logger    *zap.Logger
}

// NewVouchManager creates a new vouch manager
func NewVouchManager(db *dynamodb.Client, tableName string, signer *Signer, logger *zap.Logger) *VouchManager {
	return &VouchManager{
		db:        db,
		tableName: tableName,
		signer:    signer,
		logger:    logger,
	}
}

// CreateVouchInput contains the parameters for creating a vouch
type CreateVouchInput struct {
	FromActorID string
	ToActorID   string
	Confidence  float64
	Context     string
}

// CreateVouch creates a new vouch
func (vm *VouchManager) CreateVouch(ctx context.Context, input *CreateVouchInput) (*Vouch, error) {
	// Validate confidence
	if input.Confidence < 0 || input.Confidence > 1 {
		return nil, fmt.Errorf("confidence must be between 0 and 1")
	}

	// Check if voucher can create more vouches this month
	canVouch, err := vm.canCreateVouch(ctx, input.FromActorID)
	if err != nil {
		return nil, fmt.Errorf("failed to check vouch limit: %w", err)
	}
	if !canVouch {
		return nil, fmt.Errorf("monthly vouch limit reached")
	}

	// Get voucher's current reputation
	voucherRep, err := vm.getActorReputation(ctx, input.FromActorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher reputation: %w", err)
	}

	// Check if voucher has sufficient reputation
	if voucherRep < 500 {
		return nil, fmt.Errorf("insufficient reputation to vouch (need 500, have %d)", voucherRep)
	}

	// Create vouch
	vouch := &Vouch{
		ID:                fmt.Sprintf("vouch_%s", uuid.New().String()),
		From:              input.FromActorID,
		To:                input.ToActorID,
		InstanceURL:       vm.signer.instanceURL,
		CreatedAt:         time.Now(),
		ExpiresAt:         time.Now().Add(180 * 24 * time.Hour), // 6 months
		Confidence:        input.Confidence,
		Context:           input.Context,
		VoucherReputation: voucherRep,
		Active:            true,
		Revoked:           false,
	}

	// Sign the vouch
	if err := vm.signer.SignVouch(vouch); err != nil {
		return nil, fmt.Errorf("failed to sign vouch: %w", err)
	}

	// Store in DynamoDB
	item, err := attributevalue.MarshalMap(vouch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vouch: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouch.ID)}
	item["SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouch.ID)}
	item["GSI1PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("FROM#%s", input.FromActorID)}
	item["GSI1SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("CREATED#%s", vouch.CreatedAt.Format(time.RFC3339))}
	item["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("TO#%s", input.ToActorID)}
	item["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("CREATED#%s", vouch.CreatedAt.Format(time.RFC3339))}
	item["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", vouch.ExpiresAt.Unix())}

	_, err = vm.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(vm.tableName),
		Item:      item,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store vouch: %w", err)
	}

	vm.logger.Info("Created vouch",
		zap.String("id", vouch.ID),
		zap.String("from", input.FromActorID),
		zap.String("to", input.ToActorID),
		zap.Float64("confidence", input.Confidence))

	return vouch, nil
}

// RevokeVouch revokes an existing vouch
func (vm *VouchManager) RevokeVouch(ctx context.Context, vouchID string, actorID string) error {
	// Get the vouch
	vouch, err := vm.GetVouchByID(ctx, vouchID)
	if err != nil {
		return fmt.Errorf("failed to get vouch: %w", err)
	}

	// Check if actor can revoke (must be the voucher)
	if vouch.From != actorID {
		return fmt.Errorf("only the voucher can revoke their vouch")
	}

	// Update vouch
	now := time.Now()
	_, err = vm.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(vm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouchID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouchID)},
		},
		UpdateExpression: aws.String("SET #active = :false, #revoked = :true, #revokedAt = :now"),
		ExpressionAttributeNames: map[string]string{
			"#active":    "Active",
			"#revoked":   "Revoked",
			"#revokedAt": "RevokedAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":false": &types.AttributeValueMemberBOOL{Value: false},
			":true":  &types.AttributeValueMemberBOOL{Value: true},
			":now":   &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to revoke vouch: %w", err)
	}

	vm.logger.Info("Revoked vouch",
		zap.String("id", vouchID),
		zap.String("by", actorID))

	return nil
}

// GetVouchByID retrieves a vouch by ID
func (vm *VouchManager) GetVouchByID(ctx context.Context, vouchID string) (*Vouch, error) {
	result, err := vm.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(vm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouchID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("VOUCH#%s", vouchID)},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get vouch: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("vouch not found")
	}

	var vouch Vouch
	if err := attributevalue.UnmarshalMap(result.Item, &vouch); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vouch: %w", err)
	}

	return &vouch, nil
}

// GetVouchesForActor gets all vouches for an actor
func (vm *VouchManager) GetVouchesForActor(ctx context.Context, actorID string) ([]Vouch, error) {
	result, err := vm.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(vm.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TO#%s", actorID)},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query vouches: %w", err)
	}

	var vouches []Vouch
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &vouches); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vouches: %w", err)
	}

	// Filter out expired and revoked vouches
	activeVouches := make([]Vouch, 0)
	now := time.Now()
	for _, v := range vouches {
		if v.Active && !v.Revoked && now.Before(v.ExpiresAt) {
			activeVouches = append(activeVouches, v)
		}
	}

	return activeVouches, nil
}

// GetVouchesFromActor gets all vouches created by an actor
func (vm *VouchManager) GetVouchesFromActor(ctx context.Context, actorID string) ([]Vouch, error) {
	result, err := vm.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(vm.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("FROM#%s", actorID)},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query vouches: %w", err)
	}

	var vouches []Vouch
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &vouches); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vouches: %w", err)
	}

	return vouches, nil
}

// canCreateVouch checks if an actor can create more vouches this month
func (vm *VouchManager) canCreateVouch(ctx context.Context, actorID string) (bool, error) {
	// Get vouches created this month
	startOfMonth := time.Now().AddDate(0, 0, -time.Now().Day()+1).Truncate(24 * time.Hour)

	result, err := vm.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(vm.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND GSI1SK >= :start"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("FROM#%s", actorID)},
			":start": &types.AttributeValueMemberS{Value: fmt.Sprintf("CREATED#%s", startOfMonth.Format(time.RFC3339))},
		},
	})

	if err != nil {
		return false, fmt.Errorf("failed to query vouches: %w", err)
	}

	// Count active vouches
	activeCount := 0
	var vouches []Vouch
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &vouches); err != nil {
		return false, fmt.Errorf("failed to unmarshal vouches: %w", err)
	}

	for _, v := range vouches {
		if v.Active && !v.Revoked {
			activeCount++
		}
	}

	// Allow max 5 vouches per month
	return activeCount < 5, nil
}

// getActorReputation gets an actor's current reputation score
func (vm *VouchManager) getActorReputation(ctx context.Context, actorID string) (int, error) {
	// Query the reputation table for the latest score
	result, err := vm.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String("ReputationTable"), // TODO: Make configurable
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s", actorID)},
		},
		ScanIndexForward: aws.Bool(false), // Sort descending by SK (timestamp)
		Limit:            aws.Int32(1),
	})

	if err != nil {
		return 0, fmt.Errorf("failed to query reputation: %w", err)
	}

	if len(result.Items) == 0 {
		// No reputation history, return 0
		return 0, nil
	}

	// Unmarshal reputation data
	var repData map[string]interface{}
	if err := attributevalue.UnmarshalMap(result.Items[0], &repData); err != nil {
		return 0, fmt.Errorf("failed to unmarshal reputation: %w", err)
	}

	// Get ReputationData field
	if repDataStr, ok := repData["ReputationData"].(string); ok {
		var rep Reputation
		if err := json.Unmarshal([]byte(repDataStr), &rep); err != nil {
			return 0, fmt.Errorf("failed to unmarshal reputation data: %w", err)
		}
		return rep.TotalScore, nil
	}

	return 0, fmt.Errorf("reputation data not found")
}
