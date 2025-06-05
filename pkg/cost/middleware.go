package cost

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"go.uber.org/zap"
)

// HandlerFunc is the type for Lambda API Gateway v2 handlers
type HandlerFunc func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)

var (
	// Global cost storage instance for the middleware
	globalCostStorage *Storage
)

func init() {
	// Initialize cost storage if table name is set
	if tableName := os.Getenv("COST_HISTORY_TABLE_NAME"); tableName != "" {
		ctx := context.Background()
		cfg, err := config.LoadDefaultConfig(ctx)
		if err == nil {
			client := dynamodb.NewFromConfig(cfg)
			globalCostStorage = NewStorage(client, tableName, nil)
		}
	}
}

// Middleware wraps a handler with cost tracking
func Middleware(logger *zap.Logger) func(HandlerFunc) HandlerFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			// Create a new cost tracker for this request
			requestID := request.RequestContext.RequestID
			operationType := fmt.Sprintf("%s %s", request.RequestContext.HTTP.Method, request.RequestContext.HTTP.Path)

			tracker := NewWithRequest(requestID, operationType)

			// Track Lambda invocation
			startTime := time.Now()

			// Get Lambda memory configuration from environment
			memoryMB := int64(128) // Default
			if envMem := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE"); envMem != "" {
				if mem, err := strconv.ParseInt(envMem, 10, 64); err == nil {
					memoryMB = mem
				}
			}

			// Add tracker to context
			ctx = WithTracker(ctx, tracker)

			// Call the actual handler
			response, err := next(ctx, request)

			// Calculate Lambda duration
			duration := time.Since(startTime)
			durationMs := duration.Milliseconds()

			// Track Lambda execution
			tracker.TrackLambdaInvocation(durationMs, memoryMB)

			// Calculate response size for data transfer tracking
			if response != nil {
				// Estimate response size
				responseSize := int64(len(response.Body))
				for _, v := range response.Headers {
					responseSize += int64(len(v))
				}
				for _, v := range response.MultiValueHeaders {
					for _, mv := range v {
						responseSize += int64(len(mv))
					}
				}

				// Track data transfer
				tracker.TrackDataTransfer(responseSize)
			}

			// Calculate costs
			costs := tracker.CalculateCost()

			// Save cost data asynchronously if storage is available
			if globalCostStorage != nil && costs.TotalCostMicroCents > 0 {
				// Use a goroutine to avoid blocking the response
				// In Lambda, this will complete before the function is frozen
				go func() {
					saveCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()

					if saveErr := globalCostStorage.SaveOperationCost(saveCtx, costs); saveErr != nil {
						if logger != nil {
							logger.Error("failed to save operation cost",
								zap.String("request_id", requestID),
								zap.Error(saveErr),
							)
						}
					}
				}()
			}

			// Add cost headers to response
			if response != nil {
				if response.Headers == nil {
					response.Headers = make(map[string]string)
				}

				// Add cost headers
				response.Headers["X-Cost-Total-Microcents"] = strconv.FormatInt(costs.TotalCostMicroCents, 10)
				response.Headers["X-Cost-DynamoDB-Reads"] = strconv.FormatInt(costs.DynamoDBReads, 10)
				response.Headers["X-Cost-DynamoDB-Writes"] = strconv.FormatInt(costs.DynamoDBWrites, 10)
				response.Headers["X-Cost-Lambda-Duration-Ms"] = strconv.FormatInt(costs.LambdaDurationMs, 10)
				response.Headers["X-Cost-Data-Transfer-Bytes"] = strconv.FormatInt(costs.DataTransferBytes, 10)

				// Add cost breakdown in cents for easier reading
				if costs.TotalCostMicroCents > 0 {
					cents := float64(costs.TotalCostMicroCents) / float64(MicroCentsToCents)
					response.Headers["X-Cost-Total-Cents"] = fmt.Sprintf("%.6f", cents)
				}
			}

			// Log cost information
			if logger != nil && costs.TotalCostMicroCents > 0 {
				logger.Info("request cost tracked",
					zap.String("request_id", requestID),
					zap.String("operation", operationType),
					zap.Int64("total_microcents", costs.TotalCostMicroCents),
					zap.Float64("total_cents", float64(costs.TotalCostMicroCents)/float64(MicroCentsToCents)),
					zap.Int64("dynamodb_reads", costs.DynamoDBReads),
					zap.Int64("dynamodb_writes", costs.DynamoDBWrites),
					zap.Int64("lambda_duration_ms", costs.LambdaDurationMs),
					zap.Int64("data_transfer_bytes", costs.DataTransferBytes),
				)
			}

			return response, err
		}
	}
}

// WrapHandler wraps a handler function with cost tracking
func WrapHandler(handler HandlerFunc, logger *zap.Logger) HandlerFunc {
	return Middleware(logger)(handler)
}
