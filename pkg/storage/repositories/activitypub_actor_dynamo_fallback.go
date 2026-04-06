package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	tablesession "github.com/theory-cloud/tabletheory/pkg/session"
)

type dynamoGetItemClient interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

type repositoryDynamoSession interface {
	Client() (*dynamodb.Client, error)
}

var (
	getRepositoryConfig  = lesserconfig.Get
	newRepositorySession = func(cfg *tablesession.Config) (repositoryDynamoSession, error) {
		return tablesession.NewSession(cfg)
	}
	getRepositoryDynamoClient = defaultRepositoryDynamoClient
	repositoryDynamoClients   sync.Map
)

func defaultRepositoryDynamoClient(_ context.Context) (dynamoGetItemClient, error) {
	cfg := getRepositoryConfig()
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	sessionConfig := &tablesession.Config{
		Region:   region,
		Endpoint: cfg.DynamoDBEndpoint,
	}

	if cfg.DynamoDBEndpoint != "" {
		sessionConfig.CredentialsProvider = credentials.NewStaticCredentialsProvider("fakeMyKeyId", "fakeSecretAccessKey", "")
	}

	cacheKey := repositoryDynamoClientKey(sessionConfig.Region, sessionConfig.Endpoint)
	entryAny, _ := repositoryDynamoClients.LoadOrStore(cacheKey, &repositoryDynamoClientEntry{})
	entry := entryAny.(*repositoryDynamoClientEntry)

	entry.once.Do(func() {
		sess, err := newRepositorySession(sessionConfig)
		if err != nil {
			entry.err = fmt.Errorf("create repository dynamodb session: %w", err)
			return
		}

		client, err := sess.Client()
		if err != nil {
			entry.err = fmt.Errorf("create repository dynamodb client: %w", err)
			return
		}

		entry.client = client
	})

	if entry.err != nil {
		repositoryDynamoClients.Delete(cacheKey)
		return nil, entry.err
	}

	return entry.client, nil
}

func loadActivityPubActorFromDynamo(ctx context.Context, tableName, username string) (*activitypub.Actor, error) {
	if tableName == "" {
		return nil, fmt.Errorf("table name is empty")
	}
	if username == "" {
		return nil, fmt.Errorf("username is empty")
	}

	client, err := getRepositoryDynamoClient(ctx)
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

func loadCachedRemoteActorFromDynamo(ctx context.Context, tableName, handle string) (*activitypub.Actor, error) {
	handle = strings.TrimSpace(handle)
	if tableName == "" {
		return nil, fmt.Errorf("table name is empty")
	}
	if handle == "" {
		return nil, fmt.Errorf("remote actor handle is empty")
	}

	client, err := getRepositoryDynamoClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("init dynamodb client: %w", err)
	}

	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      &tableName,
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "REMOTE_ACTOR#" + handle},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		ProjectionExpression: awsString("actor"),
	})
	if err != nil {
		return nil, fmt.Errorf("get cached remote actor item: %w", err)
	}

	attr, ok := out.Item["actor"]
	if !ok || attr == nil {
		return nil, fmt.Errorf("cached remote actor attribute missing")
	}

	switch typed := attr.(type) {
	case *types.AttributeValueMemberM:
		var raw map[string]any
		if err := attributevalue.UnmarshalMap(typed.Value, &raw); err != nil {
			return nil, fmt.Errorf("unmarshal cached remote actor attribute map: %w", err)
		}
		return decodeActivityPubActorFromDynamoValue(raw)
	case *types.AttributeValueMemberS:
		var actor activitypub.Actor
		if err := json.Unmarshal([]byte(typed.Value), &actor); err != nil {
			return nil, fmt.Errorf("unmarshal cached remote actor attribute json: %w", err)
		}
		return &actor, nil
	default:
		return nil, fmt.Errorf("unsupported cached remote actor attribute type: %T", attr)
	}
}

func awsString(v string) *string {
	return &v
}

type repositoryDynamoClientEntry struct {
	once   sync.Once
	client dynamoGetItemClient
	err    error
}

func repositoryDynamoClientKey(region, endpoint string) string {
	return region + "|" + endpoint
}
