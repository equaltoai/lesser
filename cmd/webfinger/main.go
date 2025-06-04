package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/lesser/lesser/pkg/activitypub"
	"github.com/lesser/lesser/pkg/common"
	"github.com/lesser/lesser/pkg/config"
	"github.com/lesser/lesser/pkg/storage"
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
	// TODO: Initialize DynamoDB storage
	// store = dynamodb.New(cfg)
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
func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log := common.WithContext(ctx)

	// Extract resource parameter
	resource := request.QueryStringParameters["resource"]
	if resource == "" {
		log.Debug("missing resource parameter")
		return common.BadRequest(errors.New("missing resource parameter")), nil
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

	// TODO: Check if the actor exists in our database
	// actor, err := store.GetActor(ctx, username)
	// if err != nil {
	//     if common.IsNotFound(err) {
	//         return common.NotFound(err), nil
	//     }
	//     log.Error("failed to get actor", zap.Error(err))
	//     return common.InternalServerError(err), nil
	// }

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

func main() {
	lambda.Start(handler)
}
