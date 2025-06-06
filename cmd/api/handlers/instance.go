package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetInstanceV1 returns instance information in v1 (legacy) format
func (h *Handler) HandleGetInstanceV1(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get static config
	instanceConfig := config.GetInstanceConfig()

	// Get rules from storage
	rules, err := h.store.GetInstanceRules(ctx)
	if err != nil {
		h.logger.Warn("failed to get instance rules", zap.Error(err))
		rules = []storage.InstanceRule{}
	}

	// Get extended description
	extendedDescription, _, err := h.store.GetExtendedDescription(ctx)
	if err != nil {
		h.logger.Warn("failed to get extended description", zap.Error(err))
		extendedDescription = ""
	}

	// Get VAPID public key
	var vapidPublicKey string
	vapidKeys, err := h.store.GetVAPIDKeys(ctx)
	if err != nil {
		h.logger.Warn("failed to get VAPID keys", zap.Error(err))
		vapidPublicKey = ""
	} else {
		vapidPublicKey = vapidKeys.PublicKey
	}

	// Build v1 response (flat structure)
	resp := map[string]interface{}{
		"uri":               h.cfg.Domain,
		"title":             instanceConfig.Title,
		"short_description": instanceConfig.ShortDescription,
		"description":       instanceConfig.Description,
		"email":             instanceConfig.Email,
		"version":           instanceConfig.Version,
		"urls": map[string]interface{}{
			"streaming_api": fmt.Sprintf("wss://ws.%s", h.cfg.Domain),
		},
		"stats": map[string]interface{}{
			"user_count":   1, // TODO: Implement actual counts
			"status_count": 0,
			"domain_count": 0,
		},
		"thumbnail":         h.cfg.BaseURL() + "/assets/thumbnail.png",
		"languages":         instanceConfig.Languages,
		"registrations":     instanceConfig.RegistrationsOpen,
		"approval_required": instanceConfig.ApprovalRequired,
		"invites_enabled":   instanceConfig.InvitesEnabled,
		"contact_account":   nil, // TODO: Link to admin account

		// Configuration
		"configuration": map[string]interface{}{
			"statuses": map[string]interface{}{
				"max_characters":              instanceConfig.MaxStatusChars,
				"max_media_attachments":       4,
				"characters_reserved_per_url": 23,
			},
			"media_attachments": map[string]interface{}{
				"supported_mime_types": []string{
					"image/jpeg",
					"image/png",
					"image/gif",
					"image/webp",
					"video/mp4",
					"video/webm",
				},
				"image_size_limit": instanceConfig.MaxMediaSize,
				"video_size_limit": instanceConfig.MaxVideoSize,
			},
			"polls": map[string]interface{}{
				"max_options":               4,
				"max_characters_per_option": 50,
				"min_expiration":            300,
				"max_expiration":            2629746,
			},
		},

		// Optional fields
		"extended_description": extendedDescription,
		"vapid_key":            vapidPublicKey,
		"rules":                rules,
	}

	return common.OK(resp), nil
}

// HandleGetInstancePeers returns connected domains (federation peers)
func (h *Handler) HandleGetInstancePeers(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// TODO: Track federation peers in DynamoDB
	// For now, check if we have any remote actors in our system

	peers := []string{}

	// Get unique domains from remote actors
	// This is a simplified implementation - in production you'd want to track this separately
	actors, err := h.store.SearchAccounts(ctx, "@", 100, false, 0)
	if err != nil {
		h.logger.Warn("failed to search for remote actors", zap.Error(err))
	} else {
		domainMap := make(map[string]bool)
		for _, actor := range actors {
			// Extract domain from actor ID if it's a remote actor
			if strings.Contains(actor.ID, "https://") && !strings.Contains(actor.ID, h.cfg.Domain) {
				parts := strings.Split(actor.ID, "/")
				if len(parts) >= 3 {
					domain := strings.Replace(parts[2], "www.", "", 1)
					if domain != h.cfg.Domain {
						domainMap[domain] = true
					}
				}
			}
		}

		// Convert map to slice
		for domain := range domainMap {
			peers = append(peers, domain)
		}
	}

	return common.OK(peers), nil
}

// HandleGetInstanceActivity returns instance activity statistics
func (h *Handler) HandleGetInstanceActivity(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Generate weekly activity data for the past 12 weeks
	activity := make([]map[string]interface{}, 12)

	now := time.Now()
	for i := 0; i < 12; i++ {
		weekStart := now.AddDate(0, 0, -7*(i+1))
		weekNum := weekStart.Unix()

		// TODO: Get actual activity data from storage
		// For now, return placeholder data
		activity[11-i] = map[string]interface{}{
			"week":          fmt.Sprintf("%d", weekNum),
			"statuses":      "0",
			"logins":        "1",
			"registrations": "0",
		}
	}

	return common.OK(activity), nil
}

// HandleGetInstanceDomainBlocks returns public domain blocks
func (h *Handler) HandleGetInstanceDomainBlocks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// TODO: Implement domain blocks storage
	// For now, return empty array
	blocks := []map[string]interface{}{}

	// When implemented, each block should have:
	// {
	//   "domain": "example.com",
	//   "digest": "sha256_hash_of_domain",
	//   "severity": "suspend|silence",
	//   "comment": "Reason for block"
	// }

	return common.OK(blocks), nil
}

// HandleGetInstancePrivacyPolicy returns the privacy policy
func (h *Handler) HandleGetInstancePrivacyPolicy(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Store privacy policy content as a constant for Lambda environment
	// In production, you might want to store this in DynamoDB or S3
	const privacyPolicyContent = `# Privacy Policy

Lesser is committed to protecting your privacy. This policy explains how we handle your data.

## Data Collection
- We collect minimal data necessary for ActivityPub federation
- Account information is stored securely in DynamoDB
- Media files are stored in S3 with appropriate access controls

## Data Usage
- Your data is used solely for providing the social media service
- We do not sell or share your data with third parties
- Federation data is shared only as required by the ActivityPub protocol

## Data Retention
- Posts and media are retained until you delete them
- Account data is retained until account deletion
- Backups may retain data for up to 30 days after deletion

## Your Rights
- You can export your data at any time
- You can delete your account and all associated data
- You can control who sees your posts through privacy settings

Last updated: 2025-01-01`

	// Convert markdown to HTML
	htmlContent := h.markdownToHTML(privacyPolicyContent)

	response := map[string]interface{}{
		"content":    htmlContent,
		"updated_at": "2025-01-01T00:00:00Z",
	}

	return common.OK(response), nil
}

// HandleGetInstanceTermsOfService returns the terms of service
func (h *Handler) HandleGetInstanceTermsOfService(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Store terms of service content as a constant for Lambda environment
	// In production, you might want to store this in DynamoDB or S3
	const termsContent = `# Terms of Service

By using this Lesser instance, you agree to these terms.

## Acceptable Use
- No illegal content
- No harassment or hate speech
- No spam or commercial solicitation without permission
- Respect others' privacy and intellectual property

## Content Policy
- You retain ownership of your content
- You grant us license to federate your public posts
- We may remove content that violates these terms
- We may suspend accounts that repeatedly violate terms

## Federation
- Your public posts may be federated to other servers
- We cannot control how other servers handle your data
- You can limit federation through privacy settings

## Liability
- Service is provided "as is" without warranties
- We are not liable for data loss or service interruptions
- You are responsible for your own backups

## Changes
- We may update these terms with reasonable notice
- Continued use constitutes acceptance of new terms

Last updated: 2025-01-01`

	// Convert markdown to HTML
	htmlContent := h.markdownToHTML(termsContent)

	response := map[string]interface{}{
		"content":    htmlContent,
		"updated_at": "2025-01-01T00:00:00Z",
	}

	return common.OK(response), nil
}

// HandleGetInstanceTermsOfServiceByDate returns a specific version of the terms of service
func (h *Handler) HandleGetInstanceTermsOfServiceByDate(ctx context.Context, request events.APIGatewayV2HTTPRequest, date string) (*events.APIGatewayV2HTTPResponse, error) {
	// TODO: Implement versioned terms of service
	// For now, just return the current version
	return h.HandleGetInstanceTermsOfService(ctx, request)
}

// markdownToHTML converts markdown to HTML (basic implementation)
func (h *Handler) markdownToHTML(markdown string) string {
	// Very basic markdown to HTML conversion
	// In production, use a proper markdown parser
	html := strings.ReplaceAll(markdown, "\n\n", "</p><p>")
	html = "<p>" + html + "</p>"

	// Convert headers
	lines := strings.Split(html, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			lines[i] = "<h1>" + strings.TrimPrefix(line, "# ") + "</h1>"
		} else if strings.HasPrefix(line, "## ") {
			lines[i] = "<h2>" + strings.TrimPrefix(line, "## ") + "</h2>"
		} else if strings.HasPrefix(line, "### ") {
			lines[i] = "<h3>" + strings.TrimPrefix(line, "### ") + "</h3>"
		}
	}

	return strings.Join(lines, "\n")
}
