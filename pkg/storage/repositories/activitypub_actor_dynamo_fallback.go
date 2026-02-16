package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
)

var (
	loadDefaultAWSConfigForRepositories = awsconfig.LoadDefaultConfig

	defaultDynamoClientOnce sync.Once
	defaultDynamoClient     *dynamodb.Client
	defaultDynamoClientErr  error
)

func getDefaultDynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	defaultDynamoClientOnce.Do(func() {
		cfg := lesserconfig.Get()
		region := cfg.Region
		if region == "" {
			region = "us-east-1"
		}

		awsCfg, err := loadDefaultAWSConfigForRepositories(ctx, awsconfig.WithRegion(region))
		if err != nil {
			defaultDynamoClientErr = err
			return
		}
		defaultDynamoClient = dynamodb.NewFromConfig(awsCfg)
	})

	return defaultDynamoClient, defaultDynamoClientErr
}

func loadActivityPubActorFromDynamo(ctx context.Context, tableName, username string) (*activitypub.Actor, error) {
	if tableName == "" {
		return nil, fmt.Errorf("table name is empty")
	}
	if username == "" {
		return nil, fmt.Errorf("username is empty")
	}

	client, err := getDefaultDynamoClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("init dynamodb client: %w", err)
	}

	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      &tableName,
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "ACTOR#" + username},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		ProjectionExpression: awsString("actor"),
	})
	if err != nil {
		return nil, fmt.Errorf("get actor item: %w", err)
	}

	attr, ok := out.Item["actor"]
	if !ok || attr == nil {
		return nil, fmt.Errorf("actor attribute missing")
	}

	switch typed := attr.(type) {
	case *types.AttributeValueMemberM:
		var raw map[string]any
		if err := attributevalue.UnmarshalMap(typed.Value, &raw); err != nil {
			return nil, fmt.Errorf("unmarshal actor attribute map: %w", err)
		}
		return decodeActivityPubActorFromDynamoValue(raw)
	case *types.AttributeValueMemberS:
		var actor activitypub.Actor
		if err := json.Unmarshal([]byte(typed.Value), &actor); err != nil {
			return nil, fmt.Errorf("unmarshal actor attribute json: %w", err)
		}
		return &actor, nil
	default:
		return nil, fmt.Errorf("unsupported actor attribute type: %T", attr)
	}
}

func awsString(v string) *string {
	return &v
}
