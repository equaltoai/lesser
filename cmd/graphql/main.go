package main

import (
	"context"
	"fmt"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/aron23/lesser/graph"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/mastodon"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"go.uber.org/zap"
)

var (
	logger          *zap.Logger
	graphqlHandler  *handler.Server
	playgroundProxy *httpadapter.HandlerAdapter
	costTracker     *cost.Tracker
)

func init() {
	// Initialize logger
	logger, _ = zap.NewProduction()

	// Create storage using the package's New() function which uses global config
	storage, err := dynamodb.New()
	if err != nil {
		logger.Fatal("Failed to create storage", zap.Error(err))
	}

	// Create cost tracker
	costTracker = cost.New()

	// Create Mastodon converter
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		domain = "example.com"
	}
	mastodonConv := mastodon.NewConverter(domain)

	// Create resolver with dependencies
	resolver := &graph.Resolver{
		Storage:      storage,
		CostTracker:  costTracker,
		MastodonConv: mastodonConv,
		Logger:       logger,
	}

	// Create GraphQL server
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: resolver,
	}))

	graphqlHandler = srv

	// Create playground handler for development
	if os.Getenv("ENABLE_PLAYGROUND") == "true" {
		playgroundHandler := playground.Handler("GraphQL playground", "/graphql")
		playgroundProxy = httpadapter.New(playgroundHandler)
	}
}

// Lambda handler
func lambdaHandler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Handle GraphQL playground in development
	if request.Path == "/playground" && playgroundProxy != nil {
		return playgroundProxy.ProxyWithContext(ctx, request)
	}

	// Use httpadapter to convert Lambda event to http.Request for gqlgen
	adapter := httpadapter.New(graphqlHandler)
	response, err := adapter.ProxyWithContext(ctx, request)

	// Add cost headers
	if response.Headers == nil {
		response.Headers = make(map[string]string)
	}

	// Calculate and add cost information
	operationCost := costTracker.CalculateCost()
	response.Headers["X-Cost-Total-Micros"] = fmt.Sprintf("%d", operationCost.TotalCostMicroCents)
	response.Headers["X-Cost-DynamoDB-Reads"] = fmt.Sprintf("%d", operationCost.DynamoDBReads)
	response.Headers["X-Cost-DynamoDB-Writes"] = fmt.Sprintf("%d", operationCost.DynamoDBWrites)

	// Reset tracker for next request
	costTracker.Reset()

	return response, err
}

func main() {
	// Start Lambda runtime
	lambda.Start(lambdaHandler)
}
