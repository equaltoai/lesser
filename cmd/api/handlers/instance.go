package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	// Get real instance metrics
	userCount, err := h.store.GetTotalUserCount(ctx)
	if err != nil {
		h.logger.Warn("failed to get user count", zap.Error(err))
		userCount = 0
	}

	statusCount, err := h.store.GetTotalStatusCount(ctx)
	if err != nil {
		h.logger.Warn("failed to get status count", zap.Error(err))
		statusCount = 0
	}

	domainCount, err := h.store.GetTotalDomainCount(ctx)
	if err != nil {
		h.logger.Warn("failed to get domain count", zap.Error(err))
		domainCount = 0
	}

	// Get contact account (admin)
	var contactAccount map[string]any
	adminActor, err := h.store.GetContactAccount(ctx)
	if err != nil {
		h.logger.Warn("failed to get contact account", zap.Error(err))
	} else if adminActor != nil && adminActor.Actor != nil {
		contactAccount = map[string]any{
			"id":              adminActor.Actor.ID,
			"username":        adminActor.Actor.PreferredUsername,
			"acct":            adminActor.Actor.PreferredUsername,
			"display_name":    adminActor.Actor.Name,
			"locked":          adminActor.Actor.ManuallyApprovesFollowers,
			"bot":             false, // Default to false as Actor doesn't have Bot field
			"discoverable":    adminActor.Actor.Discoverable,
			"group":           adminActor.Actor.Type == "Group",
			"created_at":      adminActor.CreatedAt.Format(time.RFC3339),
			"note":            adminActor.Actor.Summary,
			"url":             adminActor.Actor.URL,
			"uri":             adminActor.Actor.ID,
			"avatar":          adminActor.Actor.Icon.URL,
			"avatar_static":   adminActor.Actor.Icon.URL,
			"header":          adminActor.Actor.Image.URL,
			"header_static":   adminActor.Actor.Image.URL,
			"followers_count": h.getAccountFollowersCount(ctx, adminActor.Username),
			"following_count": h.getAccountFollowingCount(ctx, adminActor.Username),
			"statuses_count":  h.getAccountStatusesCount(ctx, adminActor.Username),
			"emojis":          []any{},
			"fields":          adminActor.Fields,
		}
	}

	// Build v1 response (flat structure)
	resp := map[string]any{
		"uri":               h.cfg.Domain,
		"title":             instanceConfig.Title,
		"short_description": instanceConfig.ShortDescription,
		"description":       instanceConfig.Description,
		"email":             instanceConfig.Email,
		"version":           instanceConfig.Version,
		"urls": map[string]any{
			"streaming_api": fmt.Sprintf("wss://ws.%s/v1", h.cfg.Domain),
		},
		"stats": map[string]any{
			"user_count":   userCount,
			"status_count": statusCount,
			"domain_count": domainCount,
		},
		"thumbnail":         h.cfg.BaseURL() + "/assets/thumbnail.png",
		"languages":         instanceConfig.Languages,
		"registrations":     instanceConfig.RegistrationsOpen,
		"approval_required": instanceConfig.ApprovalRequired,
		"invites_enabled":   instanceConfig.InvitesEnabled,
		"contact_account":   contactAccount,

		// Configuration
		"configuration": map[string]any{
			"statuses": map[string]any{
				"max_characters":              instanceConfig.MaxStatusChars,
				"max_media_attachments":       4,
				"characters_reserved_per_url": 23,
			},
			"media_attachments": map[string]any{
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
			"polls": map[string]any{
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
	// Federation peers tracked in DynamoDB via federation service
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
	activity := make([]map[string]any, 12)

	now := time.Now()
	// Start from Monday of the current week
	weekStart := now.Truncate(24 * time.Hour)
	for weekStart.Weekday() != time.Monday {
		weekStart = weekStart.Add(-24 * time.Hour)
	}

	// Get activity for each of the past 12 weeks
	for i := 0; i < 12; i++ {
		// Calculate the start of each week (going backwards)
		thisWeekStart := weekStart.AddDate(0, 0, -7*i)
		weekTimestamp := thisWeekStart.Unix()

		// Get activity data from storage
		weekActivity, err := h.store.GetWeeklyActivity(ctx, weekTimestamp)
		if err != nil {
			h.logger.Warn("failed to get weekly activity",
				zap.Int64("week", weekTimestamp),
				zap.Error(err))
			// Use zero values on error
			weekActivity = &storage.WeeklyActivity{
				Week:          weekTimestamp,
				Statuses:      0,
				Logins:        0,
				Registrations: 0,
			}
		}

		// Format for API response (newest week first)
		activity[i] = map[string]any{
			"week":          fmt.Sprintf("%d", weekTimestamp),
			"statuses":      fmt.Sprintf("%d", weekActivity.Statuses),
			"logins":        fmt.Sprintf("%d", weekActivity.Logins),
			"registrations": fmt.Sprintf("%d", weekActivity.Registrations),
		}
	}

	return common.OK(activity), nil
}

// HandleGetInstanceDomainBlocks returns public domain blocks
func (h *Handler) HandleGetInstanceDomainBlocks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get domain blocks from storage
	domainBlocks, _, err := h.store.ListInstanceDomainBlocks(ctx, 100, "")
	if err != nil {
		h.logger.Warn("failed to get domain blocks", zap.Error(err))
		// Return empty array on error
		return common.OK([]map[string]any{}), nil
	}

	// Convert to API format
	blocks := make([]map[string]any, 0, len(domainBlocks))
	for _, block := range domainBlocks {
		// Only include public blocks (not obfuscated)
		if !block.Obfuscate && block.PublicComment != "" {
			// Create SHA256 hash of domain for digest
			hash := sha256.Sum256([]byte(block.Domain))
			digest := hex.EncodeToString(hash[:])

			blocks = append(blocks, map[string]any{
				"domain":   block.Domain,
				"digest":   digest,
				"severity": block.Severity,
				"comment":  block.PublicComment,
			})
		}
	}

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

	response := map[string]any{
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

	response := map[string]any{
		"content":    htmlContent,
		"updated_at": "2025-01-01T00:00:00Z",
	}

	return common.OK(response), nil
}

// HandleGetInstanceTermsOfServiceByDate returns a specific version of the terms of service
func (h *Handler) HandleGetInstanceTermsOfServiceByDate(ctx context.Context, request events.APIGatewayV2HTTPRequest, date string) (*events.APIGatewayV2HTTPResponse, error) {
	// Versioned terms of service not implemented - returning current version
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

// Helper methods for getting account statistics
func (h *Handler) getAccountFollowersCount(ctx context.Context, username string) int {
	count, err := h.store.GetFollowersCount(ctx, username)
	if err != nil {
		h.logger.Warn("failed to get followers count", zap.Error(err))
		return 0
	}
	return count
}

func (h *Handler) getAccountFollowingCount(ctx context.Context, username string) int {
	count, err := h.store.GetFollowingCount(ctx, username)
	if err != nil {
		h.logger.Warn("failed to get following count", zap.Error(err))
		return 0
	}
	return count
}

func (h *Handler) getAccountStatusesCount(ctx context.Context, username string) int {
	count, err := h.store.GetUserStatusCount(ctx, username)
	if err != nil {
		h.logger.Warn("failed to get statuses count", zap.Error(err))
		return 0
	}
	return count
}
