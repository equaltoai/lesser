package cost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInitializeCostTracking(t *testing.T) {
	request := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-request-123",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/api/users",
			},
		},
	}

	tracker, requestID, operationType := initializeCostTracking(request)

	require.NotNil(t, tracker)
	assert.Equal(t, "test-request-123", requestID)
	assert.Equal(t, "GET /api/users", operationType)
}

func TestGetLambdaMemoryConfig(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		memoryMB := getLambdaMemoryConfig()
		assert.Equal(t, int64(128), memoryMB)
	})

	t.Run("parses env variable", func(t *testing.T) {
		t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "512")
		memoryMB := getLambdaMemoryConfig()
		assert.Equal(t, int64(512), memoryMB)
	})

	t.Run("returns default for invalid env", func(t *testing.T) {
		t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "invalid")
		memoryMB := getLambdaMemoryConfig()
		assert.Equal(t, int64(128), memoryMB)
	})
}

func TestTrackLambdaExecution(t *testing.T) {
	tracker := New()
	tracker.circuitBreaker = nil
	startTime := time.Now().Add(-100 * time.Millisecond)
	memoryMB := int64(256)

	trackLambdaExecution(tracker, startTime, memoryMB)

	assert.Equal(t, int64(1), tracker.lambdaInvocations.Load())
	assert.True(t, tracker.lambdaDurationMs.Load() >= 100)
	assert.Equal(t, int64(256), tracker.lambdaMemoryMB.Load())
}

func TestTrackResponseDataTransfer(t *testing.T) {
	tracker := New()
	response := &events.APIGatewayV2HTTPResponse{
		Body: "Hello, World!",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}

	trackResponseDataTransfer(tracker, response)

	// Body length + header value length
	expectedSize := int64(len("Hello, World!") + len("application/json"))
	assert.Equal(t, expectedSize, tracker.dataTransfer.Load())
}

func TestCalculateResponseSize(t *testing.T) {
	t.Run("calculates body size", func(t *testing.T) {
		response := &events.APIGatewayV2HTTPResponse{
			Body: "Hello, World!",
		}

		size := calculateResponseSize(response)
		assert.Equal(t, int64(13), size)
	})

	t.Run("includes headers", func(t *testing.T) {
		response := &events.APIGatewayV2HTTPResponse{
			Body: "Hi",
			Headers: map[string]string{
				"Content-Type": "text/plain",
			},
		}

		size := calculateResponseSize(response)
		assert.Equal(t, int64(2+10), size) // "Hi" + "text/plain"
	})

	t.Run("includes multi-value headers", func(t *testing.T) {
		response := &events.APIGatewayV2HTTPResponse{
			Body: "",
			MultiValueHeaders: map[string][]string{
				"Set-Cookie": {"cookie1", "cookie2"},
			},
		}

		size := calculateResponseSize(response)
		assert.Equal(t, int64(7+7), size) // "cookie1" + "cookie2"
	})
}

func TestCalculateHeadersSize(t *testing.T) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"X-Custom":     "value",
	}

	size := calculateHeadersSize(headers)
	assert.Equal(t, int64(len("application/json")+len("value")), size)
}

func TestCalculateMultiValueHeadersSize(t *testing.T) {
	headers := map[string][]string{
		"Set-Cookie": {"cookie1", "cookie2", "cookie3"},
	}

	size := calculateMultiValueHeadersSize(headers)
	assert.Equal(t, int64(7+7+7), size) // 3 cookies of 7 chars each
}

func TestAddCostHeaders(t *testing.T) {
	t.Run("adds headers to nil map", func(t *testing.T) {
		response := &events.APIGatewayV2HTTPResponse{}
		costs := &OperationCost{
			TotalCostMicroCents: 1000,
			DynamoDBReads:       10,
			DynamoDBWrites:      5,
			LambdaDurationMs:    100,
			DataTransferBytes:   500,
		}

		addCostHeaders(response, costs)

		require.NotNil(t, response.Headers)
		assert.Equal(t, "1000", response.Headers["X-Cost-Total-Microcents"])
		assert.Equal(t, "10", response.Headers["X-Cost-DynamoDB-Reads"])
		assert.Equal(t, "5", response.Headers["X-Cost-DynamoDB-Writes"])
		assert.Equal(t, "100", response.Headers["X-Cost-Lambda-Duration-Ms"])
		assert.Equal(t, "500", response.Headers["X-Cost-Data-Transfer-Bytes"])
	})

	t.Run("adds cents header when cost > 0", func(t *testing.T) {
		response := &events.APIGatewayV2HTTPResponse{}
		costs := &OperationCost{
			TotalCostMicroCents: 1000000, // 1 cent
		}

		addCostHeaders(response, costs)

		assert.Contains(t, response.Headers, "X-Cost-Total-Cents")
	})

	t.Run("does not add cents header when cost is 0", func(t *testing.T) {
		response := &events.APIGatewayV2HTTPResponse{}
		costs := &OperationCost{
			TotalCostMicroCents: 0,
		}

		addCostHeaders(response, costs)

		assert.NotContains(t, response.Headers, "X-Cost-Total-Cents")
	})
}

func TestAddBasicCostHeaders(t *testing.T) {
	headers := make(map[string]string)
	costs := &OperationCost{
		TotalCostMicroCents: 5000,
		DynamoDBReads:       20,
		DynamoDBWrites:      10,
		LambdaDurationMs:    200,
		DataTransferBytes:   1024,
	}

	addBasicCostHeaders(headers, costs)

	assert.Equal(t, "5000", headers["X-Cost-Total-Microcents"])
	assert.Equal(t, "20", headers["X-Cost-DynamoDB-Reads"])
	assert.Equal(t, "10", headers["X-Cost-DynamoDB-Writes"])
	assert.Equal(t, "200", headers["X-Cost-Lambda-Duration-Ms"])
	assert.Equal(t, "1024", headers["X-Cost-Data-Transfer-Bytes"])
}

func TestAddCentsHeader(t *testing.T) {
	t.Run("adds header when cost > 0", func(t *testing.T) {
		headers := make(map[string]string)
		costs := &OperationCost{
			TotalCostMicroCents: 1000000, // 1 cent
		}

		addCentsHeader(headers, costs)

		assert.Contains(t, headers, "X-Cost-Total-Cents")
		assert.Equal(t, "1.000000", headers["X-Cost-Total-Cents"])
	})

	t.Run("does not add header when cost is 0", func(t *testing.T) {
		headers := make(map[string]string)
		costs := &OperationCost{
			TotalCostMicroCents: 0,
		}

		addCentsHeader(headers, costs)

		assert.NotContains(t, headers, "X-Cost-Total-Cents")
	})
}

func TestLogCostInformation(t *testing.T) {
	t.Run("logs when cost > 0", func(t *testing.T) {
		logger := zap.NewNop()
		costs := &OperationCost{
			TotalCostMicroCents: 1000,
			DynamoDBReads:       10,
			DynamoDBWrites:      5,
			LambdaDurationMs:    100,
			DataTransferBytes:   500,
		}

		// Should not panic
		logCostInformation(logger, "req-123", "GET /api/users", costs)
	})

	t.Run("does not log when cost is 0", func(t *testing.T) {
		logger := zap.NewNop()
		costs := &OperationCost{
			TotalCostMicroCents: 0,
		}

		// Should not panic
		logCostInformation(logger, "req-123", "GET /api/users", costs)
	})

	t.Run("handles nil logger", func(t *testing.T) {
		costs := &OperationCost{
			TotalCostMicroCents: 1000,
		}

		// Should not panic
		logCostInformation(nil, "req-123", "GET /api/users", costs)
	})
}

func TestMiddleware(t *testing.T) {
	logger := zap.NewNop()

	handler := func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		// Simulate some work
		tracker := FromContext(ctx)
		if tracker != nil {
			tracker.circuitBreaker = nil
			_ = tracker.TrackDynamoRead(5)
		}

		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Body:       "OK",
		}, nil
	}

	wrappedHandler := Middleware(logger)(handler)

	request := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-123",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/test",
			},
		},
	}

	response, err := wrappedHandler(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, 200, response.StatusCode)
	assert.Contains(t, response.Headers, "X-Cost-Total-Microcents")
}

func TestWrapHandler(t *testing.T) {
	logger := zap.NewNop()

	handler := func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Body:       "OK",
		}, nil
	}

	wrappedHandler := WrapHandler(handler, logger)

	request := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-456",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "POST",
				Path:   "/api/data",
			},
		},
	}

	response, err := wrappedHandler(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, 200, response.StatusCode)
}

func TestSaveCostWithRetry(t *testing.T) {
	t.Run("does nothing when storage is nil", func(t *testing.T) {
		// Reset global storage
		originalStorage := globalCostStorage
		globalCostStorage = nil
		defer func() { globalCostStorage = originalStorage }()

		logger := zap.NewNop()
		cost := &OperationCost{
			RequestID:           "req-123",
			TotalCostMicroCents: 1000,
		}

		// Should not panic
		saveCostWithRetry(context.Background(), cost, logger)
	})

	t.Run("does nothing when cost is 0", func(t *testing.T) {
		logger := zap.NewNop()
		cost := &OperationCost{
			RequestID:           "req-123",
			TotalCostMicroCents: 0,
		}

		// Should not panic
		saveCostWithRetry(context.Background(), cost, logger)
	})
}

func TestMiddleware_NilResponse(t *testing.T) {
	logger := zap.NewNop()

	handler := func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return nil, nil
	}

	wrappedHandler := Middleware(logger)(handler)

	request := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-789",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/test",
			},
		},
	}

	response, err := wrappedHandler(context.Background(), request)

	require.NoError(t, err)
	assert.Nil(t, response)
}

func TestCostBuffer(t *testing.T) {
	// Test that the cost buffer variables exist and have expected defaults
	assert.Equal(t, 100, maxBufferSize)
}

func TestMiddleware_HandlerError(t *testing.T) {
	logger := zap.NewNop()

	handler := func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return nil, errors.New("handler error")
	}

	wrappedHandler := Middleware(logger)(handler)

	request := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-error",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/test",
			},
		},
	}

	response, err := wrappedHandler(context.Background(), request)

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestMiddleware_WithExistingHeaders(t *testing.T) {
	logger := zap.NewNop()

	handler := func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Body:       "OK",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil
	}

	wrappedHandler := Middleware(logger)(handler)

	request := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RequestID: "test-headers",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "POST",
				Path:   "/api/data",
			},
		},
	}

	response, err := wrappedHandler(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "application/json", response.Headers["Content-Type"])
	assert.Contains(t, response.Headers, "X-Cost-Total-Microcents")
}
