package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/reputation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg        *config.Config
	store      storage.Storage
	logger     *zap.Logger
	repService *reputation.Service
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	// Initialize DynamoDB storage
	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Initialize reputation service
	repService, err = reputation.NewService(&reputation.Config{
		Storage:     store,
		Logger:      logger,
		InstanceURL: cfg.BaseURL(),
		PrivateKey:  cfg.ReputationPrivateKey,
	})
	if err != nil {
		logger.Fatal("failed to initialize reputation service", zap.Error(err))
	}
}

// parseWebFingerResource parses a WebFinger resource identifier
// Expected format: acct:username@domain
func parseWebFingerResource(resource string) (username, domain string, err error) {
	// Validate webfinger format using comprehensive validation
	if err := activitypub.ValidateWebfinger(resource); err != nil {
		return "", "", common.ValidationError{
			Field:   "resource",
			Message: err.Error(),
		}
	}

	// Extract username and domain
	acct := strings.TrimPrefix(resource, "acct:")
	parts := strings.Split(acct, "@")
	username = parts[0]
	domain = parts[1]

	return username, domain, nil
}

// handler processes WebFinger requests
// GET /.well-known/webfinger?resource=acct:username@domain
func handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Remove stage prefix if present
	path := request.RawPath
	if request.RequestContext.Stage != "" && strings.HasPrefix(path, "/"+request.RequestContext.Stage) {
		path = strings.TrimPrefix(path, "/"+request.RequestContext.Stage)
	}

	log := logger.With(
		zap.String("path", path),
		zap.Any("query", request.QueryStringParameters),
		zap.Any("headers", request.Headers),
	)

	// Handle different endpoints
	switch path {
	case "/.well-known/webfinger":
		return handleWebFinger(ctx, request, log)
	case "/.well-known/nodeinfo":
		return handleNodeInfoDiscovery(ctx, request, log)
	case "/.well-known/reputation-keys":
		return handleReputationKeys(ctx, request, log)
	case "/nodeinfo/2.0":
		return handleNodeInfo20(ctx, request, log)
	case "/nodeinfo/2.1":
		return handleNodeInfo21(ctx, request, log)
	default:
		log.Debug("unknown endpoint", zap.String("path", path))
		return common.NotFound(fmt.Errorf("unknown endpoint: %s", path)), nil
	}
}

// handleWebFinger handles webfinger requests
func handleWebFinger(ctx context.Context, request events.APIGatewayV2HTTPRequest, log *zap.Logger) (*events.APIGatewayV2HTTPResponse, error) {
	// Validate resource parameter
	resource := request.QueryStringParameters["resource"]
	if resource == "" {
		log.Warn("missing resource parameter")
		return common.BadRequest(common.ValidationError{Field: "resource", Message: "resource parameter is required"}), nil
	}

	log.Info("processing webfinger request",
		zap.String("resource", resource),
	)

	// Parse the resource
	username, domain, err := parseWebFingerResource(resource)
	if err != nil {
		log.Debug("invalid resource format",
			zap.String("resource", resource),
			zap.Error(err),
		)
		return common.ErrorFromType(err), nil
	}

	// Check if the domain matches our instance
	if domain != cfg.Domain {
		log.Debug("domain mismatch",
			zap.String("requested_domain", domain),
			zap.String("our_domain", cfg.Domain),
		)
		return common.NotFound(common.ActorNotFoundError{Username: username}), nil
	}

	// Check if the actor exists in our database
	_, err = store.GetActor(ctx, username)
	if err != nil {
		if common.IsNotFound(err) {
			log.Debug("actor not found",
				zap.String("username", username),
			)
			return common.NotFound(err), nil
		}
		log.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Build WebFinger response
	actorURL := cfg.ActorURL(username)
	response := activitypub.WebFingerResource{
		Subject: resource,
		Aliases: []string{actorURL},
		Links: []activitypub.WebFingerLink{
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: actorURL,
			},
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: actorURL,
			},
			{
				Rel:      "http://ostatus.org/schema/1.0/subscribe",
				Template: fmt.Sprintf("%s/authorize_interaction?uri={uri}", cfg.BaseURL()),
			},
		},
	}

	log.Info("webfinger request successful",
		zap.String("username", username),
	)

	// Return WebFinger response with proper content type
	resp := common.JSONResponse(200, response)
	resp.Headers["Content-Type"] = "application/jrd+json"
	resp.Headers["Cache-Control"] = "max-age=86400" // Cache for 24 hours

	return resp, nil
}

// handleNodeInfoDiscovery returns the well-known nodeinfo discovery document
func handleNodeInfoDiscovery(ctx context.Context, request events.APIGatewayV2HTTPRequest, log *zap.Logger) (*events.APIGatewayV2HTTPResponse, error) {
	// Explicitly ignore unused parameters
	_ = ctx
	_ = request

	log.Info("handling nodeinfo discovery request")

	discovery := map[string]interface{}{
		"links": []map[string]string{
			{
				"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.0",
				"href": cfg.BaseURL() + "/nodeinfo/2.0",
			},
			{
				"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.1",
				"href": cfg.BaseURL() + "/nodeinfo/2.1",
			},
		},
	}

	resp := common.JSONResponse(200, discovery)
	resp.Headers["Cache-Control"] = "max-age=86400" // Cache for 24 hours
	return resp, nil
}

// handleNodeInfo20 returns nodeinfo 2.0 format
func handleNodeInfo20(ctx context.Context, request events.APIGatewayV2HTTPRequest, log *zap.Logger) (*events.APIGatewayV2HTTPResponse, error) {
	// Explicitly ignore unused parameters
	_ = request

	log.Info("handling nodeinfo 2.0 request")

	// Get actual user count from storage
	userCount64, err := store.GetTotalUserCount(ctx)
	if err != nil {
		log.Warn("failed to get user count, using default", zap.Error(err))
		userCount64 = 1
	}
	userCount := int(userCount64)

	nodeinfo := map[string]interface{}{
		"version": "2.0",
		"software": map[string]interface{}{
			"name":    "lesser",
			"version": "0.1.0",
		},
		"protocols": []string{"activitypub"},
		"services": map[string]interface{}{
			"outbound": []string{},
			"inbound":  []string{},
		},
		"usage": map[string]interface{}{
			"users": map[string]interface{}{
				"total": userCount,
			},
			"localPosts": func() int {
				postCount64, err := store.GetTotalStatusCount(ctx)
				if err != nil {
					log.Warn("failed to get post count, using default", zap.Error(err))
					return 0
				}
				return int(postCount64)
			}(),
		},
		"openRegistrations": true,
		"metadata": map[string]interface{}{
			"nodeName":        cfg.InstanceName,
			"nodeDescription": "A serverless ActivityPub implementation",
		},
	}

	resp := common.JSONResponse(200, nodeinfo)
	resp.Headers["Content-Type"] = "application/json; profile=\"http://nodeinfo.diaspora.software/ns/schema/2.0#\""
	resp.Headers["Cache-Control"] = "max-age=300" // Cache for 5 minutes
	return resp, nil
}

// handleNodeInfo21 returns nodeinfo 2.1 format
func handleNodeInfo21(ctx context.Context, request events.APIGatewayV2HTTPRequest, log *zap.Logger) (*events.APIGatewayV2HTTPResponse, error) {
	// Explicitly ignore unused parameters
	_ = request

	log.Info("handling nodeinfo 2.1 request")

	// Get actual user count from storage
	userCount64, err := store.GetTotalUserCount(ctx)
	if err != nil {
		log.Warn("failed to get user count, using default", zap.Error(err))
		userCount64 = 1
	}
	userCount := int(userCount64)

	nodeinfo := map[string]interface{}{
		"version": "2.1",
		"software": map[string]interface{}{
			"name":       "lesser",
			"version":    "0.1.0",
			"repository": "https://github.com/aron23/lesser",
		},
		"protocols": []string{"activitypub"},
		"services": map[string]interface{}{
			"outbound": []string{},
			"inbound":  []string{},
		},
		"usage": map[string]interface{}{
			"users": map[string]interface{}{
				"total": userCount,
				"activeMonth": func() int {
					activeCount, err := store.GetActiveUserCount(ctx, 30)
					if err != nil {
						log.Warn("failed to get active user count, using total", zap.Error(err))
						return userCount
					}
					return int(activeCount)
				}(),
				"activeHalfyear": func() int {
					activeCount, err := store.GetActiveUserCount(ctx, 180)
					if err != nil {
						log.Warn("failed to get active user count, using total", zap.Error(err))
						return userCount
					}
					return int(activeCount)
				}(),
			},
			"localPosts": func() int {
				postCount64, err := store.GetTotalStatusCount(ctx)
				if err != nil {
					log.Warn("failed to get post count, using default", zap.Error(err))
					return 0
				}
				return int(postCount64)
			}(),
		},
		"openRegistrations": true,
		"metadata": map[string]interface{}{
			"nodeName":        cfg.InstanceName,
			"nodeDescription": "A serverless ActivityPub implementation",
		},
	}

	resp := common.JSONResponse(200, nodeinfo)
	resp.Headers["Content-Type"] = "application/json; profile=\"http://nodeinfo.diaspora.software/ns/schema/2.1#\""
	resp.Headers["Cache-Control"] = "max-age=300" // Cache for 5 minutes
	return resp, nil
}

// handleReputationKeys returns the instance's public key for reputation signing
func handleReputationKeys(ctx context.Context, request events.APIGatewayV2HTTPRequest, log *zap.Logger) (*events.APIGatewayV2HTTPResponse, error) {
	// Explicitly ignore unused parameters
	_ = ctx
	_ = request

	log.Info("handling reputation keys request")

	// Get the actual public key from the reputation service
	publicKeyBase64 := repService.GetPublicKey()
	if publicKeyBase64 == "" {
		log.Error("reputation service returned empty public key")
		return common.InternalServerError(fmt.Errorf("reputation service unavailable")), nil
	}

	keys := map[string]interface{}{
		"publicKey": publicKeyBase64,
		"algorithm": "Ed25519",
		"keyId":     cfg.BaseURL() + "#reputation-key",
		"created":   time.Now().UTC().Format(time.RFC3339),
	}

	log.Debug("returning reputation keys",
		zap.String("keyId", keys["keyId"].(string)),
		zap.String("publicKey", publicKeyBase64[:20]+"..."))

	resp := common.JSONResponse(200, keys)
	resp.Headers["Cache-Control"] = "max-age=3600" // Cache for 1 hour
	return resp, nil
}

func main() {
	lambda.Start(handler)
}
