package main

import (
	"context"
	"fmt"
	"net/http"
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

	// Initialize storage
	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}
}

func handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	log := common.WithContext(ctx)

	// Extract username from path
	username := request.PathParameters["username"]
	if username == "" {
		return common.BadRequest(common.ValidationError{Field: "username", Message: "missing username"}), nil
	}

	log.Info("fetching actor profile",
		zap.String("username", username),
		zap.String("accept", request.Headers["Accept"]))

	// Get actor from storage
	actor, err := store.GetActor(ctx, username)
	if err != nil {
		if common.IsNotFound(err) {
			return common.NotFound(err), nil
		}
		log.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Content negotiation
	accept := request.Headers["Accept"]
	if accept == "" {
		accept = request.Headers["accept"] // Try lowercase
	}

	// Check if client wants ActivityStreams JSON
	if strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json") ||
		strings.Contains(accept, "application/json") {
		// Return ActivityStreams JSON
		return common.ActivityPubResponse(http.StatusOK, actor), nil
	}

	// Return HTML for browsers
	html := generateHTMLProfile(actor)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "text/html; charset=utf-8",
		},
		Body: html,
	}, nil
}

func generateHTMLProfile(actor *activitypub.Actor) string {
	// Extract display name or fall back to username
	displayName := actor.Name
	if displayName == "" {
		displayName = actor.PreferredUsername
	}

	// Build social media meta tags for better sharing
	metaTags := fmt.Sprintf(`
		<meta property="og:type" content="profile">
		<meta property="og:title" content="%s">
		<meta property="og:description" content="%s">
		<meta property="og:url" content="%s">`,
		displayName,
		actor.BaseObject.Summary,
		actor.BaseObject.ID)

	if actor.Icon != nil && actor.Icon.URL != "" {
		metaTags += fmt.Sprintf(`
		<meta property="og:image" content="%s">`, actor.Icon.URL)
	}

	// Generate followers/following counts if available
	statsHTML := ""
	if actor.Followers != "" && actor.Following != "" {
		statsHTML = fmt.Sprintf(`
		<div class="stats">
			<p><a href="%s">Followers</a> | <a href="%s">Following</a></p>
		</div>`, actor.Followers, actor.Following)
	}

	// Build the HTML page
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>%s (@%s@%s)</title>
	%s
	<link rel="alternate" type="application/activity+json" href="%s">
	<style>
		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
			max-width: 600px;
			margin: 40px auto;
			padding: 20px;
			line-height: 1.6;
			color: #333;
			background-color: #f5f5f5;
		}
		.profile {
			background: white;
			border-radius: 8px;
			padding: 30px;
			box-shadow: 0 2px 4px rgba(0,0,0,0.1);
		}
		.avatar {
			width: 100px;
			height: 100px;
			border-radius: 50%%;
			margin-bottom: 20px;
		}
		h1 {
			margin: 0 0 10px 0;
			font-size: 24px;
		}
		.username {
			color: #666;
			font-size: 16px;
			margin-bottom: 20px;
		}
		.bio {
			margin-bottom: 20px;
		}
		.stats {
			border-top: 1px solid #eee;
			padding-top: 20px;
			margin-top: 20px;
		}
		.stats a {
			color: #0066cc;
			text-decoration: none;
			margin-right: 20px;
		}
		.stats a:hover {
			text-decoration: underline;
		}
		.meta {
			margin-top: 20px;
			padding-top: 20px;
			border-top: 1px solid #eee;
			font-size: 14px;
			color: #666;
		}
		.meta a {
			color: #0066cc;
			text-decoration: none;
		}
	</style>
</head>
<body>
	<div class="profile">`,
		displayName, actor.PreferredUsername, cfg.Domain,
		metaTags,
		actor.BaseObject.ID)

	// Add avatar if available
	if actor.Icon != nil && actor.Icon.URL != "" {
		html += fmt.Sprintf(`
		<img src="%s" alt="%s" class="avatar">`, actor.Icon.URL, displayName)
	}

	// Add profile content
	html += fmt.Sprintf(`
		<h1>%s</h1>
		<div class="username">@%s@%s</div>`, displayName, actor.PreferredUsername, cfg.Domain)

	// Add bio if available
	if actor.BaseObject.Summary != "" {
		html += fmt.Sprintf(`
		<div class="bio">%s</div>`, actor.BaseObject.Summary)
	}

	// Add stats if available
	html += statsHTML

	// Add ActivityPub discovery info
	html += fmt.Sprintf(`
		<div class="meta">
			<p>This is an ActivityPub profile. You can follow @%s@%s from any compatible server.</p>
			<p><a href="%s" type="application/activity+json">View ActivityPub data</a></p>
		</div>`, actor.PreferredUsername, cfg.Domain, actor.BaseObject.ID)

	html += `
	</div>
</body>
</html>`

	return html
}

func main() {
	lambda.Start(handler)
}
