package handlers

import (
	"fmt"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

// HandleNodeInfoWellKnownLift handles /.well-known/nodeinfo requests
func (h *Handler) HandleNodeInfoWellKnownLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	h.logger.Info("nodeinfo well-known request",
		zap.String("user_agent", headerValue(ctx, "User-Agent")))

	response := apimodels.NodeInfoWellKnown{
		Links: []apimodels.NodeInfoLink{
			{
				Rel:  "http://nodeinfo.diaspora.software/ns/schema/2.0",
				Href: fmt.Sprintf("%s/nodeinfo/2.0", h.cfg.BaseURL()),
			},
		},
	}

	return okJSON(response)
}

// HandleNodeInfoLift handles /nodeinfo/2.0 requests
func (h *Handler) HandleNodeInfoLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	h.logger.Info("nodeinfo request",
		zap.String("user_agent", headerValue(ctx, "User-Agent")))

	// Get instance configuration - using default values in test environment
	var instanceConfig *config.InstanceConfig
	if h.cfg.Domain == "example.com" {
		// Test environment - use defaults
		softwareVersion := config.NormalizeLesserVersion(h.cfg.Version)
		instanceConfig = &config.InstanceConfig{
			Title:             "Test Instance",
			ShortDescription:  "A test instance",
			Description:       "Test instance description",
			Email:             "admin@example.com",
			Version:           config.MastodonCompatibleVersion(softwareVersion),
			Software:          "lesser",
			SoftwareVersion:   softwareVersion,
			Languages:         []string{"en"},
			MaxStatusChars:    500,
			MaxMediaSize:      10485760,  // 10MB
			MaxVideoSize:      104857600, // 100MB
			RegistrationsOpen: true,
			FederationEnabled: true,
		}
	} else {
		// Production - get from config
		instanceConfig = config.GetInstanceConfig()
	}

	// Get instance statistics using Accounts service
	stats, err := h.registry.Accounts().GetInstanceStats(ctx.Context(), &accounts.GetInstanceStatsQuery{})
	if err != nil {
		h.logger.Warn("failed to get instance stats", zap.Error(err))
		// Use defaults
		stats = &accounts.GetInstanceStatsResult{
			TotalUsers:     1,
			ActiveMonth:    1,
			ActiveHalfyear: 1,
			LocalPosts:     0,
			LocalComments:  0,
		}
	}

	response := apimodels.NodeInfo{
		Version: "2.0",
		Software: apimodels.NodeInfoSoftware{
			Name:       "lesser",
			Version:    config.NormalizeLesserVersion(instanceConfig.SoftwareVersion),
			Repository: "https://github.com/equaltoai/lesser",
			Homepage:   "https://github.com/equaltoai/lesser",
		},
		Protocols: []string{
			"activitypub",
		},
		Services: apimodels.NodeInfoServices{
			Inbound:  []string{},
			Outbound: []string{},
		},
		OpenRegistrations: instanceConfig.RegistrationsOpen,
		Usage: apimodels.NodeInfoUsage{
			Users: apimodels.NodeInfoUsers{
				Total:          stats.TotalUsers,
				ActiveHalfyear: stats.ActiveHalfyear,
				ActiveMonth:    stats.ActiveMonth,
			},
			LocalPosts:    stats.LocalPosts,
			LocalComments: stats.LocalComments,
		},
		Metadata: map[string]any{
			"nodeName":        instanceConfig.Title,
			"nodeDescription": instanceConfig.Description,
			"maintainer": map[string]any{
				"name":  instanceConfig.Email,
				"email": instanceConfig.Email,
			},
			"langs":                     instanceConfig.Languages,
			"tosUrl":                    fmt.Sprintf("%s/terms", h.cfg.BaseURL()),
			"privacyPolicyUrl":          fmt.Sprintf("%s/privacy-policy", h.cfg.BaseURL()),
			"registrationMessage":       "",
			"shortDescription":          instanceConfig.Description,
			"email":                     instanceConfig.Email,
			"accountActivationRequired": instanceConfig.ApprovalRequired,
			"invitesEnabled":            false,
			"configuration": map[string]any{
				"statuses": map[string]any{
					"max_characters": instanceConfig.MaxStatusChars,
				},
				"media_attachments": map[string]any{
					"image_size_limit": instanceConfig.MaxMediaSize,
					"video_size_limit": instanceConfig.MaxVideoSize,
				},
			},
		},
	}

	return okJSON(response)
}
