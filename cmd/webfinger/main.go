package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg    *config.Config
	store  storage.Storage
	logger *zap.Logger
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
}

// parseWebFingerResource parses a WebFinger resource identifier
// Expected format: acct:username@domain
func parseWebFingerResource(resource string) (username, domain string, err error) {
	if !strings.HasPrefix(resource, "acct:") {
		return "", "", common.ValidationError{
			Field:   "resource",
			Message: "must start with 'acct:'",
		}
	}

	acct := strings.TrimPrefix(resource, "acct:")
	parts := strings.Split(acct, "@")
	if len(parts) != 2 {
		return "", "", common.ValidationError{
			Field:   "resource",
			Message: "invalid format, expected acct:username@domain",
		}
	}

	username = parts[0]
	domain = parts[1]

	if username == "" {
		return "", "", common.ValidationError{
			Field:   "resource",
			Message: "username cannot be empty",
		}
	}

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
	log.Info("handling nodeinfo 2.0 request")

	// TODO: implement user counting
	userCount := 1

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
			"localPosts": 0, // TODO: implement post counting
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
	log.Info("handling nodeinfo 2.1 request")

	// TODO: implement user counting
	userCount := 1

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
				"total":          userCount,
				"activeMonth":    userCount, // TODO: implement proper active user counting
				"activeHalfyear": userCount,
			},
			"localPosts": 0, // TODO: implement post counting
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

func main() {
	lambda.Start(handler)
}
