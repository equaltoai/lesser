package handlers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
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
func (h *Handler) HandleWebFingerLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get resource parameter
	resource := queryValue(ctx, "resource")

	if err := common.ValidateRequiredParam("resource", resource); err != nil {
		return common.RespondBadRequest(ctx, "resource parameter is required")
	}

	// Validate webfinger resource format
	if err := common.ValidateWebfingerResource(resource); err != nil {
		h.logger.Warn("invalid webfinger resource format",
			zap.String("resource", resource),
			zap.Error(err))
		return common.RespondBadRequest(ctx, err.Error())
	}

	h.logger.Info("webfinger request",
		zap.String("resource", resource),
		zap.String("user_agent", headerValue(ctx, "User-Agent")))

	// Parse the resource
	username, domain, err := h.parseWebFingerResourceLift(resource)
	if err != nil {
		h.logger.Warn("invalid webfinger resource",
			zap.String("resource", resource),
			zap.Error(err))
		return common.RespondBadRequest(ctx, fmt.Sprintf("invalid resource format: %v", err))
	}

	// Verify this is for our domain
	if domain != h.cfg.Domain {
		h.logger.Warn("webfinger request for wrong domain",
			zap.String("requested_domain", domain),
			zap.String("our_domain", h.cfg.Domain))
		return common.RespondNotFound(ctx, "user not found")
	}

	// Look up the user using Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context(), username)
	if err != nil {
		h.logger.Warn("account not found for webfinger",
			zap.String("username", username),
			zap.Error(err))
		return common.RespondNotFound(ctx, "user not found")
	}

	// Get the actor from the account
	actor := account.Actor
	if actor == nil {
		h.logger.Warn("actor data missing for account",
			zap.String("username", username))
		return common.RespondNotFound(ctx, "user not found")
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

	return okJSON(response)
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
	if err := common.ValidateMultipleRequiredParams(map[string]string{"username": username, "domain": domain}); err != nil {
		return "", "", errors.New("username and domain cannot be empty")
	}

	return username, domain, nil
}
