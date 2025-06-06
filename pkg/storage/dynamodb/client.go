package dynamodb

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/aron23/lesser/pkg/common"
	cfg "github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// DynamoDBAPI defines the subset of DynamoDB operations we use
type DynamoDBAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
}

// dynamoDBStorage implements the storage.Storage interface using DynamoDB
type dynamoDBStorage struct {
	client        DynamoDBAPI
	tableName     string
	searchService *SearchService
}

var (
	// globalClient is reused across Lambda invocations
	globalClient DynamoDBAPI
	clientOnce   sync.Once
	clientErr    error
)

// init initializes the global DynamoDB client for Lambda reuse
func init() {
	// Skip initialization in test mode
	if os.Getenv("GO_ENV") == "test" {
		return
	}

	// Pre-initialize the client in Lambda environment
	if cfg.Get().DynamoTableName != "" {
		_, _ = getClient()
	}
}

// getClient returns the global DynamoDB client, initializing it if needed
func getClient() (DynamoDBAPI, error) {
	clientOnce.Do(func() {
		ctx := context.Background()
		awsCfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Get().Region),
		)
		if err != nil {
			clientErr = fmt.Errorf("failed to load AWS config: %w", err)
			return
		}

		// Create base DynamoDB client
		baseClient := dynamodb.NewFromConfig(awsCfg)

		// Wrap with cost tracking if not in test mode
		if os.Getenv("GO_ENV") != "test" && os.Getenv("DISABLE_COST_TRACKING") != "true" {
			globalClient = cost.NewDynamoDBWrapper(baseClient)
			common.Logger().Info("DynamoDB client initialized with cost tracking",
				zap.String("region", cfg.Get().Region),
			)
		} else {
			globalClient = baseClient
			common.Logger().Info("DynamoDB client initialized",
				zap.String("region", cfg.Get().Region),
			)
		}
	})

	return globalClient, clientErr
}

// New creates a new DynamoDB storage instance
func New() (storage.Storage, error) {
	client, err := getClient()
	if err != nil {
		return nil, err
	}

	tableName := cfg.Get().DynamoTableName
	dynStorage := &dynamoDBStorage{
		client:    client,
		tableName: tableName,
	}

	// Initialize search service with storage reference
	// For now, search service needs the actual DynamoDB client
	// TODO: Update search service to use the DynamoDBAPI interface
	if dc, ok := client.(*dynamodb.Client); ok {
		searchService := NewSearchService(dc, tableName, common.Logger(), dynStorage, cfg.Get().Domain)
		dynStorage.searchService = searchService
	}

	return dynStorage, nil
}

// NewWithClient creates a new DynamoDB storage instance with a custom client (for testing)
func NewWithClient(client DynamoDBAPI, tableName string) storage.Storage {
	dynStorage := &dynamoDBStorage{
		client:    client,
		tableName: tableName,
	}

	// For testing, we might not have a real DynamoDB client
	if dynamoClient, ok := client.(*dynamodb.Client); ok {
		searchService := NewSearchService(dynamoClient, tableName, common.Logger(), dynStorage, cfg.Get().Domain)
		dynStorage.searchService = searchService
	}

	return dynStorage
}

// getTableName returns the table name with optional override for testing
func (s *dynamoDBStorage) getTableName() *string {
	return aws.String(s.tableName)
}

// DynamoDB Attribute Value Conversion Utilities

// ConvertFromDynamoDB recursively converts DynamoDB attribute values to plain Go types
func ConvertFromDynamoDB(av interface{}) interface{} {
	switch v := av.(type) {
	case map[string]interface{}:
		// Check if this is a DynamoDB attribute value
		if isDynamoDBAttributeValue(v) {
			return extractDynamoDBValue(v)
		}
		// Otherwise, recursively convert the map
		result := make(map[string]interface{})
		for key, value := range v {
			result[key] = ConvertFromDynamoDB(value)
		}
		return result
	case []interface{}:
		// Recursively convert slice elements
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = ConvertFromDynamoDB(item)
		}
		return result
	default:
		// Return as-is for basic types
		return v
	}
}

// isDynamoDBAttributeValue checks if a map represents a DynamoDB attribute value
func isDynamoDBAttributeValue(m map[string]interface{}) bool {
	// DynamoDB attribute values have exactly one key that matches a type
	if len(m) != 1 {
		return false
	}
	for key := range m {
		switch key {
		case "S", "N", "B", "SS", "NS", "BS", "M", "L", "NULL", "BOOL":
			return true
		}
	}
	return false
}

// extractDynamoDBValue extracts the actual value from a DynamoDB attribute value
func extractDynamoDBValue(av map[string]interface{}) interface{} {
	for key, value := range av {
		switch key {
		case "S":
			// String
			if s, ok := value.(string); ok {
				return s
			}
		case "N":
			// Number (return as string to preserve precision)
			if n, ok := value.(string); ok {
				return n
			}
		case "BOOL":
			// Boolean
			if b, ok := value.(bool); ok {
				return b
			}
		case "NULL":
			// Null
			return nil
		case "M":
			// Map
			if m, ok := value.(map[string]interface{}); ok {
				return ConvertFromDynamoDB(m)
			}
		case "L":
			// List
			if l, ok := value.([]interface{}); ok {
				return ConvertFromDynamoDB(l)
			}
		case "SS":
			// String Set
			if ss, ok := value.([]interface{}); ok {
				result := make([]string, len(ss))
				for i, s := range ss {
					if str, ok := s.(string); ok {
						result[i] = str
					}
				}
				return result
			}
		case "NS":
			// Number Set
			if ns, ok := value.([]interface{}); ok {
				result := make([]string, len(ns))
				for i, n := range ns {
					if str, ok := n.(string); ok {
						result[i] = str
					}
				}
				return result
			}
		case "BS":
			// Binary Set
			return value
		case "B":
			// Binary
			return value
		}
	}
	return nil
}

// ConvertToDynamoDB converts plain Go types to DynamoDB attribute value format
func ConvertToDynamoDB(v interface{}) interface{} {
	if v == nil {
		return map[string]interface{}{"NULL": true}
	}

	switch val := v.(type) {
	case string:
		return map[string]interface{}{"S": val}
	case int, int32, int64, uint, uint32, uint64, float32, float64:
		return map[string]interface{}{"N": fmt.Sprintf("%v", val)}
	case bool:
		return map[string]interface{}{"BOOL": val}
	case []byte:
		return map[string]interface{}{"B": val}
	case map[string]interface{}:
		m := make(map[string]interface{})
		for k, v := range val {
			m[k] = ConvertToDynamoDB(v)
		}
		return map[string]interface{}{"M": m}
	case []interface{}:
		l := make([]interface{}, len(val))
		for i, item := range val {
			l[i] = ConvertToDynamoDB(item)
		}
		return map[string]interface{}{"L": l}
	case []string:
		if len(val) > 0 {
			return map[string]interface{}{"SS": val}
		}
		return map[string]interface{}{"L": []interface{}{}}
	default:
		// For unknown types, try to convert to string
		return map[string]interface{}{"S": fmt.Sprintf("%v", val)}
	}
}

// UnmarshalItem is a wrapper around attributevalue.UnmarshalMap that handles DynamoDB format conversion
func (s *dynamoDBStorage) UnmarshalItem(item map[string]types.AttributeValue, out interface{}) error {
	// First, do the standard unmarshal
	if err := attributevalue.UnmarshalMap(item, out); err != nil {
		// If standard unmarshal fails, try converting from DynamoDB format first
		plainMap := make(map[string]interface{})
		for k, v := range item {
			plainMap[k] = attributeValueToInterface(v)
		}

		// Apply our conversion to handle nested DynamoDB formats
		convertedMap := ConvertFromDynamoDB(plainMap)

		// Try to marshal to JSON then unmarshal to the target type
		jsonBytes, jsonErr := json.Marshal(convertedMap)
		if jsonErr != nil {
			return fmt.Errorf("unmarshal failed: %w, json conversion also failed: %v", err, jsonErr)
		}

		if unmarshalErr := json.Unmarshal(jsonBytes, out); unmarshalErr != nil {
			return fmt.Errorf("unmarshal failed: %w, json unmarshal also failed: %v", err, unmarshalErr)
		}

		return nil
	}
	return nil
}

// MarshalItem is a wrapper around attributevalue.MarshalMap for consistency
func (s *dynamoDBStorage) MarshalItem(in interface{}) (map[string]types.AttributeValue, error) {
	return attributevalue.MarshalMap(in)
}

// UnmarshalListOfMaps converts a list of DynamoDB items to a slice of the target type
func (s *dynamoDBStorage) UnmarshalListOfMaps(items []map[string]types.AttributeValue, out interface{}) error {
	// Use reflection to ensure 'out' is a pointer to a slice
	outValue := reflect.ValueOf(out)
	if outValue.Kind() != reflect.Ptr || outValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("out must be a pointer to a slice")
	}

	sliceValue := outValue.Elem()
	sliceType := sliceValue.Type()
	elementType := sliceType.Elem()

	// Create a new slice with the right capacity
	newSlice := reflect.MakeSlice(sliceType, 0, len(items))

	for _, item := range items {
		// Create a new instance of the element type
		newElem := reflect.New(elementType)

		// Unmarshal into it
		if err := s.UnmarshalItem(item, newElem.Interface()); err != nil {
			// Log error but continue with other items
			common.Logger().Error("failed to unmarshal item in list",
				zap.Error(err),
				zap.Any("item", item))
			continue
		}

		// Append to slice
		newSlice = reflect.Append(newSlice, newElem.Elem())
	}

	// Set the result
	sliceValue.Set(newSlice)
	return nil
}

// attributeValueToInterface converts a DynamoDB AttributeValue to a Go interface{}
// This is used internally by UnmarshalItem
func attributeValueToInterface(av types.AttributeValue) interface{} {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return v.Value
	case *types.AttributeValueMemberBOOL:
		return v.Value
	case *types.AttributeValueMemberNULL:
		return nil
	case *types.AttributeValueMemberM:
		m := make(map[string]interface{})
		for k, val := range v.Value {
			m[k] = attributeValueToInterface(val)
		}
		return m
	case *types.AttributeValueMemberL:
		l := make([]interface{}, len(v.Value))
		for i, val := range v.Value {
			l[i] = attributeValueToInterface(val)
		}
		return l
	case *types.AttributeValueMemberSS:
		return v.Value
	case *types.AttributeValueMemberNS:
		return v.Value
	case *types.AttributeValueMemberBS:
		return v.Value
	case *types.AttributeValueMemberB:
		return v.Value
	default:
		return nil
	}
}

// GetCollection is implemented in collection.go

// Helper methods for key construction
func (s *dynamoDBStorage) userPK(username string) string {
	return "USER#" + username
}
