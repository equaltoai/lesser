package lift

import (
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/pay-theory/lift/pkg/lift"
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

// HandleWebFingerLift handles /.well-known/webfinger requests
func (h *Handler) HandleWebFingerLift(ctx *lift.Context) error {
	// Get resource parameter
	resource := ctx.Query("resource")

	// Fallback to direct query param access if ctx.Query doesn't work
	if resource == "" && ctx.Request != nil && ctx.Request.Request != nil {
		resource = ctx.Request.Request.QueryParams["resource"]
	}

	if resource == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "resource parameter is required"})
	}

	h.logger.Info("webfinger request",
		zap.String("resource", resource),
		zap.String("user_agent", ctx.Header("User-Agent")))

	// Parse the resource
	username, domain, err := h.parseWebFingerResourceLift(resource)
	if err != nil {
		h.logger.Warn("invalid webfinger resource",
			zap.String("resource", resource),
			zap.Error(err))
		return ctx.Status(400).JSON(map[string]string{"error": fmt.Sprintf("invalid resource format: %v", err)})
	}

	// Verify this is for our domain
	if domain != h.cfg.Domain {
		h.logger.Warn("webfinger request for wrong domain",
			zap.String("requested_domain", domain),
			zap.String("our_domain", h.cfg.Domain))
		return ctx.Status(404).JSON(map[string]string{"error": "user not found"})
	}

	// Look up the user
	actor, err := h.repos.Actor().GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Warn("actor not found for webfinger",
			zap.String("username", username),
			zap.Error(err))
		return ctx.Status(404).JSON(map[string]string{"error": "user not found"})
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

	return ctx.JSON(response)
}

// parseWebFingerResourceLift parses a WebFinger resource identifier
func (h *Handler) parseWebFingerResourceLift(resource string) (username, domain string, err error) {
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
