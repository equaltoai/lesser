package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// WebFingerResponse represents a WebFinger response
type WebFingerResponse struct {
	Subject string          `json:"subject"`
	Aliases []string        `json:"aliases,omitempty"`
	Links   []WebFingerLink `json:"links"`
	Props   map[string]any  `json:"properties,omitempty"`
}

// WebFingerLink represents a link in a WebFinger response
type WebFingerLink struct {
	Rel        string            `json:"rel"`
	Type       string            `json:"type,omitempty"`
	Href       string            `json:"href,omitempty"`
	Template   string            `json:"template,omitempty"`
	Titles     map[string]string `json:"titles,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
}

// HandleWebFinger handles /.well-known/webfinger requests
func (h *Handler) HandleWebFinger(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get resource parameter
	resource := request.QueryStringParameters["resource"]
	if resource == "" {
		return common.BadRequest(errors.New("resource parameter is required")), nil
	}

	h.logger.Info("webfinger request",
		zap.String("resource", resource),
		zap.String("user_agent", request.Headers["user-agent"]))

	// Parse the resource
	username, domain, err := h.parseWebFingerResource(resource)
	if err != nil {
		h.logger.Warn("invalid webfinger resource",
			zap.String("resource", resource),
			zap.Error(err))
		return common.BadRequest(fmt.Errorf("invalid resource format: %w", err)), nil
	}

	// Verify this is for our domain
	if domain != h.cfg.Domain {
		h.logger.Warn("webfinger request for wrong domain",
			zap.String("requested_domain", domain),
			zap.String("our_domain", h.cfg.Domain))
		return common.NotFound(errors.New("user not found")), nil
	}

	// Look up the user
	actor, err := h.store.GetActor(ctx, username)
	if err != nil {
		h.logger.Warn("actor not found for webfinger",
			zap.String("username", username),
			zap.Error(err))
		return common.NotFound(errors.New("user not found")), nil
	}

	// Build WebFinger response
	subject := fmt.Sprintf("acct:%s@%s", username, domain)
	actorURL := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)

	response := WebFingerResponse{
		Subject: subject,
		Aliases: []string{
			actorURL,
		},
		Links: []WebFingerLink{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: actorURL,
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: actorURL,
			},
			{
				Rel:      "http://schemas.google.com/g/2010#updates-from",
				Type:     "application/atom+xml",
				Template: fmt.Sprintf("%s/users/%s/feed.atom", h.cfg.BaseURL(), username),
			},
		},
	}

	// Add avatar link if available
	if actor.Icon != nil && actor.Icon.URL != "" {
		response.Links = append(response.Links, WebFingerLink{
			Rel:  "http://webfinger.net/rel/avatar",
			Type: "image/jpeg", // Default, could be detected from URL
			Href: actor.Icon.URL,
		})
	}

	return common.OK(response), nil
}

// parseWebFingerResource parses a WebFinger resource identifier
func (h *Handler) parseWebFingerResource(resource string) (username, domain string, err error) {
	// Validate webfinger format
	if err := activitypub.ValidateWebfinger(resource); err != nil {
		return "", "", err
	}

	// Extract username and domain
	acct := strings.TrimPrefix(resource, "acct:")
	parts := strings.Split(acct, "@")
	if len(parts) != 2 {
		return "", "", errors.New("invalid account format")
	}

	username = parts[0]
	domain = parts[1]

	// Basic validation
	if username == "" || domain == "" {
		return "", "", errors.New("username and domain cannot be empty")
	}

	return username, domain, nil
}
