package main

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/dynamorm"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/common"
)

var (
	db        *dynamorm.LambdaDB
	tableName string
	handler   *ActivityHandler
)

func init() {
	// Logger is already initialized in common package init()

	// Get table name from environment
	tableName = os.Getenv("DYNAMO_TABLE_NAME")
	if tableName == "" {
		common.Logger().Fatal("DYNAMO_TABLE_NAME environment variable is required")
	}

	// Initialize DynamORM with Lambda optimization
	var err error
	db, err = dynamorm.NewLambdaOptimized()
	if err != nil {
		common.Logger().Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Set timeout buffer to prevent Lambda timeouts
	if lambdaDB, ok := db.WithLambdaTimeoutBuffer(500 * time.Millisecond).(*dynamorm.LambdaDB); ok {
		db = lambdaDB
	}

	// Create activity handler
	handler = NewActivityHandler(db, tableName)
}

// handleDynamoDBStream processes DynamoDB stream events for activities
func handleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error {
	log := common.WithContext(ctx)
	log.Info("Processing DynamoDB stream event", zap.Int("records", len(event.Records)))

	// Use the handler to process the event
	return handler.HandleDynamoDBStream(ctx, event)
}

func main() {
	lambda.Start(handleDynamoDBStream)
}
