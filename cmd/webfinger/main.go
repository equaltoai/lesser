package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/lesser/lesser/pkg/activitypub"
	"github.com/lesser/lesser/pkg/config"
	"github.com/lesser/lesser/pkg/storage"
)

var (
	cfg   *config.Config
	store storage.Storage
)

func init() {
	cfg = config.Get()
	// TODO: Initialize DynamoDB storage
	// store = dynamodb.New(cfg)
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// WebFinger endpoint should handle requests like:
	// GET /.well-known/webfinger?resource=acct:username@domain

	resource := request.QueryStringParameters["resource"]
	if resource == "" {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"error": "Missing resource parameter"}`,
		}, nil
	}

	// Parse the resource parameter
	// Expected format: acct:username@domain
	if !strings.HasPrefix(resource, "acct:") {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"error": "Resource must start with 'acct:'"}`,
		}, nil
	}

	acct := strings.TrimPrefix(resource, "acct:")
	parts := strings.Split(acct, "@")
	if len(parts) != 2 {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"error": "Invalid resource format"}`,
		}, nil
	}

	username := parts[0]
	domain := parts[1]

	// Check if the domain matches our instance
	if domain != cfg.Domain {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusNotFound,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"error": "User not found"}`,
		}, nil
	}

	// TODO: Check if the actor exists in our database
	// For now, we'll create a mock response

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

	body, err := json.Marshal(response)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"error": "Internal server error"}`,
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":                "application/jrd+json",
			"Access-Control-Allow-Origin": "*",
			"Cache-Control":               "max-age=86400", // Cache for 24 hours
		},
		Body: string(body),
	}, nil
}

func main() {
	lambda.Start(handler)
}
