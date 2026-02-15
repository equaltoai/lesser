package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const (
	// EnvProduction represents production environment name
	EnvProduction = "production"
	// EnvProd represents short production environment name
	EnvProd = "prod"
)

// HandleGetInstanceV1Lift returns instance information in v1 (legacy) format
func (h *Handler) HandleGetInstanceV1Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get static config
	instanceConfig := config.GetInstanceConfig()

	state, stateErr := h.repos.Instance().GetInstanceState(ctx.Context())
	locked := stateErr != nil || state.Locked

	// Get rules from storage
	rules, err := h.repos.Instance().GetInstanceRules(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get instance rules", zap.Error(err))
		rules = []storage.InstanceRule{}
	}

	// Get extended description
	extendedDescription, _, err := h.repos.Instance().GetExtendedDescription(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get extended description", zap.Error(err))
		extendedDescription = ""
	}

	// Get VAPID public key
	var vapidPublicKey string
	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context())
	if err != nil {
		// Check if we're in production mode
		env := h.cfg.Stage
		if env == EnvProduction || env == EnvProd {
			// In production, VAPID keys are required for push notifications
			h.logger.Error("VAPID keys are required in production but not found", zap.Error(err))
			return common.RespondInternalServerError(ctx, "VAPID keys not configured - push notifications unavailable")
		}

		h.logger.Warn("failed to get VAPID keys", zap.Error(err))
		vapidPublicKey = ""
	} else {
		vapidPublicKey = vapidKeys.PublicKey
	}

	// Get real instance metrics
	userCount, err := h.repos.Analytics().GetTotalUserCount(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get user count", zap.Error(err))
		userCount = 0
	}

	statusCount, err := h.repos.Instance().GetTotalStatusCount(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get status count", zap.Error(err))
		statusCount = 0
	}

	domainCount, err := h.repos.Instance().GetTotalDomainCount(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get domain count", zap.Error(err))
		domainCount = 0
	}

	// Get contact account (admin)
	var contactAccount map[string]any
	adminActor, err := h.repos.Instance().GetContactAccount(ctx.Context())
	if err != nil {
		h.logger.Warn("failed to get contact account", zap.Error(err))
	} else if adminActor != nil {
		contactAccount = map[string]any{
			"id":              adminActor.ID,
			"username":        adminActor.Username,
			"acct":            adminActor.Username,
			"display_name":    adminActor.DisplayName,
			"locked":          false, // Default to false as ActorRecord doesn't have this field
			"bot":             false, // Default to false as ActorRecord doesn't have Bot field
			"discoverable":    true,  // Default to true as ActorRecord doesn't have this field
			"group":           adminActor.ActorType == actorTypeGroup,
			"created_at":      adminActor.CreatedAt.Format(time.RFC3339),
			"note":            "", // ActorRecord doesn't have summary
			"url":             fmt.Sprintf("https://%s/@%s", h.cfg.Domain, adminActor.Username),
			"uri":             adminActor.ID,
			"avatar":          adminActor.Avatar,
			"avatar_static":   adminActor.Avatar,
			"header":          "", // ActorRecord doesn't have header
			"header_static":   "", // ActorRecord doesn't have header
			"followers_count": h.getAccountFollowersCountLift(ctx.Context(), adminActor.Username),
			"following_count": h.getAccountFollowingCountLift(ctx.Context(), adminActor.Username),
			"statuses_count":  h.getAccountStatusesCountLift(ctx.Context(), adminActor.Username),
			"emojis":          []any{},
			"fields":          []any{}, // ActorRecord doesn't have fields
		}
	}

	resp := apimodels.InstanceV1Response{
		URI:              h.cfg.Domain,
		Title:            instanceConfig.Title,
		ShortDescription: instanceConfig.ShortDescription,
		Description:      instanceConfig.Description,
		Email:            instanceConfig.Email,
		Version:          instanceConfig.Version,
		URLs: map[string]any{
			"streaming_api": h.streamingAPIURL(),
		},
		Stats: map[string]any{
			"user_count":   userCount,
			"status_count": statusCount,
			"domain_count": domainCount,
		},
		Thumbnail:        h.cfg.BaseURL() + "/assets/thumbnail.png",
		Languages:        instanceConfig.Languages,
		Registrations:    instanceConfig.RegistrationsOpen && !locked,
		ApprovalRequired: instanceConfig.ApprovalRequired,
		InvitesEnabled:   instanceConfig.InvitesEnabled,
		ContactAccount:   contactAccount,
		Configuration: map[string]any{
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
			"tips": func() map[string]any {
				enabled := false
				chainID := 0
				contractAddress := ""
				if h.cfg != nil {
					enabled = h.cfg.TipEnabled
					chainID = h.cfg.TipChainID
					contractAddress = strings.TrimSpace(h.cfg.TipContractAddress)
				}

				if enabled && (chainID == 0 || contractAddress == "") {
					h.logger.Warn("tips enabled but missing chain ID or contract address; disabling tips in instance config",
						zap.Int("chain_id", chainID),
						zap.String("contract_address", contractAddress))
					enabled = false
				}

				out := map[string]any{
					"enabled": enabled,
				}
				if enabled {
					out["chain_id"] = chainID
					out["contract_address"] = contractAddress
				}
				return out
			}(),
			"translation": map[string]any{
				"enabled": h.cfg != nil && h.cfg.TranslationEnabled,
			},
		},
		ExtendedDescription: extendedDescription,
		VAPIDKey:            vapidPublicKey,
		Rules:               rules,
	}

	return okJSON(resp)
}

// HandleGetInstancePeersLift returns connected domains (federation peers)
func (h *Handler) HandleGetInstancePeersLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Federation peers tracked in DynamoDB via federation service
	// For now, check if we have any remote actors in our system

	peers := []string{}

	// Get unique domains from remote actors
	// This is a simplified implementation - in production you'd want to track this separately
	actors, err := h.repos.Search().SearchAccounts(ctx.Context(), "@", 100, false, 0)
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

	return okJSON(peers)
}

// HandleGetInstanceActivityLift returns instance activity statistics
func (h *Handler) HandleGetInstanceActivityLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Generate weekly activity data for the past 12 weeks
	activity := make([]apimodels.InstanceActivityEntry, 12)

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
		weekActivity, err := h.repos.Instance().GetWeeklyActivity(ctx.Context(), weekTimestamp)
		if err != nil {
			h.logger.Warn("failed to get weekly activity",
				zap.Int64("week", weekTimestamp),
				zap.Error(err))
			// Use zero values on error
			weekActivity = &storage.WeeklyActivity{
				Week:          fmt.Sprintf("%d", weekTimestamp),
				Statuses:      0,
				Logins:        0,
				Registrations: 0,
			}
		}

		// Format for API response (newest week first)
		activity[i] = apimodels.InstanceActivityEntry{
			Week:          fmt.Sprintf("%d", weekTimestamp),
			Statuses:      fmt.Sprintf("%d", weekActivity.Statuses),
			Logins:        fmt.Sprintf("%d", weekActivity.Logins),
			Registrations: fmt.Sprintf("%d", weekActivity.Registrations),
		}
	}

	return okJSON(activity)
}

// HandleGetInstanceDomainBlocksLift returns public domain blocks
func (h *Handler) HandleGetInstanceDomainBlocksLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get domain blocks from storage
	domainBlocks, _, err := h.repos.DomainBlock().ListInstanceDomainBlocks(ctx.Context(), 100, "")
	if err != nil {
		h.logger.Warn("failed to get domain blocks", zap.Error(err))
		// Return empty array on error
		return okJSON([]apimodels.InstanceDomainBlock{})
	}

	// Convert to API format
	blocks := make([]apimodels.InstanceDomainBlock, 0, len(domainBlocks))
	for _, block := range domainBlocks {
		// Only include public blocks (not obfuscated)
		if !block.Obfuscate && block.PublicComment != "" {
			// Create SHA256 hash of domain for digest
			hash := sha256.Sum256([]byte(block.Domain))
			digest := hex.EncodeToString(hash[:])

			blocks = append(blocks, apimodels.InstanceDomainBlock{
				Domain:   block.Domain,
				Digest:   digest,
				Severity: block.Severity,
				Comment:  block.PublicComment,
			})
		}
	}

	return okJSON(blocks)
}

// HandleGetInstancePrivacyPolicyLift returns the privacy policy
func (h *Handler) HandleGetInstancePrivacyPolicyLift(_ *apptheory.Context) (*apptheory.Response, error) {
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
	htmlContent := h.markdownToHTMLLift(privacyPolicyContent)

	response := map[string]any{
		"content":    htmlContent,
		"updated_at": "2025-01-01T00:00:00Z",
	}

	return okJSON(response)
}

// HandleGetInstanceTermsOfServiceLift returns the terms of service
func (h *Handler) HandleGetInstanceTermsOfServiceLift(_ *apptheory.Context) (*apptheory.Response, error) {
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
	htmlContent := h.markdownToHTMLLift(termsContent)

	response := map[string]any{
		"content":    htmlContent,
		"updated_at": "2025-01-01T00:00:00Z",
	}

	return okJSON(response)
}

// markdownToHTMLLift converts markdown to HTML (basic implementation)
func (h *Handler) markdownToHTMLLift(markdown string) string {
	// Very basic markdown to HTML conversion
	// In production, use a proper markdown parser

	lines := strings.Split(markdown, "\n")
	var processedLines []string
	var inParagraph bool

	for _, line := range lines {
		trimmed := common.SanitizeInput(line)

		// Process each line and update paragraph state
		result, newParagraphState := h.processMarkdownLine(line, trimmed, inParagraph)
		if result != "" {
			processedLines = append(processedLines, result)
		}
		inParagraph = newParagraphState
	}

	// Close any open paragraph
	if inParagraph {
		processedLines = append(processedLines, "</p>")
	}

	return strings.Join(processedLines, "\n")
}

// processMarkdownLine processes a single line of markdown and returns HTML
func (h *Handler) processMarkdownLine(line, trimmed string, inParagraph bool) (string, bool) {
	// Check for header
	if header := h.convertMarkdownHeader(trimmed); header != "" {
		if inParagraph {
			return "</p>\n" + header, false
		}
		return header, false
	}

	// Handle empty lines
	if err := common.ValidateRequiredParam("trimmed_line", trimmed); err != nil {
		if inParagraph {
			return "</p>", false
		}
		return "", false
	}

	// Handle regular text
	if !inParagraph {
		return "<p>" + line, true
	}
	return line, true
}

// convertMarkdownHeader converts markdown headers to HTML
func (h *Handler) convertMarkdownHeader(trimmed string) string {
	headerPrefixes := map[string]string{
		"### ": "h3",
		"## ":  "h2",
		"# ":   "h1",
	}

	// Check prefixes in order (longest first to avoid false matches)
	for prefix, tag := range headerPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			content := strings.TrimPrefix(trimmed, prefix)
			return fmt.Sprintf("<%s>%s</%s>", tag, content, tag)
		}
	}

	return ""
}

// streamingAPIURL returns the WebSocket streaming endpoint for the Mastodon instance API.
// Mastodon clients use this URL to establish WebSocket connections for real-time updates.
func (h *Handler) streamingAPIURL() string {
	if h == nil || h.cfg == nil {
		return ""
	}

	if endpoint := strings.TrimSpace(h.cfg.WebSocketEndpoint); endpoint != "" {
		switch {
		case strings.HasPrefix(endpoint, "wss://"), strings.HasPrefix(endpoint, "ws://"):
			return endpoint
		case strings.HasPrefix(endpoint, "https://"):
			return "wss://" + strings.TrimPrefix(endpoint, "https://")
		case strings.HasPrefix(endpoint, "http://"):
			return "ws://" + strings.TrimPrefix(endpoint, "http://")
		}
	}

	domain := strings.TrimSpace(h.cfg.Domain)
	scheme := "wss"
	if domain == "localhost" || domain == "127.0.0.1" {
		scheme = "ws"
	}
	return fmt.Sprintf("%s://ws.%s", scheme, domain)
}

// Helper methods for getting account statistics
func (h *Handler) getAccountFollowersCountLift(ctx context.Context, username string) int {
	count, err := h.repos.Relationship().CountFollowers(ctx, username)
	if err != nil {
		h.logger.Warn("failed to get followers count", zap.Error(err))
		return 0
	}
	return count
}

func (h *Handler) getAccountFollowingCountLift(ctx context.Context, username string) int {
	count, err := h.repos.Relationship().CountFollowing(ctx, username)
	if err != nil {
		h.logger.Warn("failed to get following count", zap.Error(err))
		return 0
	}
	return count
}

func (h *Handler) getAccountStatusesCountLift(ctx context.Context, username string) int {
	count, err := h.repos.Object().GetUserStatusCount(ctx, username)
	if err != nil {
		h.logger.Warn("failed to get statuses count", zap.Error(err))
		return 0
	}
	return count
}
