package main

/*
Lesser API Server - Mastodon-compatible ActivityPub implementation

This Lambda function serves the Lesser API using AWS API Gateway v2.
All routing is handled by the chi router defined in router.go.

The API Gateway configuration strips the /api/v1 and /api/v2 prefixes
before passing requests to this Lambda, so the router receives clean paths.

For debugging, see the detailed logging in lambdaHandler which captures
request details including path transformations and base64 decoding.
*/

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/aron23/lesser/cmd/api/handlers"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage"
	storageDB "github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg            *config.Config
	store          storage.Storage
	logger         *zap.Logger
	handler        *handlers.Handler
	authMiddleware *auth.Middleware
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	var err error
	store, err = storageDB.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Initialize auth middleware
	authMiddleware, err = auth.GetMiddleware()
	if err != nil {
		logger.Fatal("failed to initialize auth middleware", zap.Error(err))
	}

	// Create handler with all dependencies
	handler = handlers.NewHandler(cfg, store, logger, authMiddleware)
}

func lambdaHandler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Log POST /statuses requests for debugging
	if request.RequestContext.HTTP.Method == "POST" && strings.Contains(request.RequestContext.HTTP.Path, "statuses") {
		logger.Info("lambdaHandler received POST /statuses",
			zap.String("body", request.Body),
			zap.Bool("is_base64", request.IsBase64Encoded),
			zap.Any("headers", request.Headers),
			zap.Int("body_length", len(request.Body)),
			zap.Any("request_context", request.RequestContext),
			zap.Any("raw_path", request.RawPath),
			zap.Any("raw_query", request.RawQueryString))

		// Log the entire request struct for debugging
		requestJSON, _ := json.Marshal(request)
		logger.Info("Full request JSON", zap.String("request", string(requestJSON)))

		// Try base64 decode even if flag is false
		if request.Body != "" {
			decoded, err := base64.StdEncoding.DecodeString(request.Body)
			if err == nil {
				logger.Info("Base64 decode succeeded",
					zap.String("decoded", string(decoded)))
			} else {
				logger.Info("Base64 decode failed",
					zap.Error(err))
			}
		}
	}

	// Create chi router
	router := NewRouter(handler, *authMiddleware, logger)

	// Use the router-based handler with cost tracking
	routerHandler := LambdaHandlerWithRouter(router)
	return cost.WrapHandler(routerHandler, logger)(ctx, request)
}

func main() {
	lambda.Start(lambdaHandler)
}
