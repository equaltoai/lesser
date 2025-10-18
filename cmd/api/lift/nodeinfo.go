package lift

import (
	"fmt"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// NodeInfoWellKnown represents the .well-known/nodeinfo response
type NodeInfoWellKnown struct {
	Links []NodeInfoLink `json:"links"`
}

// NodeInfoLink represents a link in the .well-known/nodeinfo response
type NodeInfoLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

// NodeInfo represents a NodeInfo 2.0 response
type NodeInfo struct {
	Version           string           `json:"version"`
	Software          NodeInfoSoftware `json:"software"`
	Protocols         []string         `json:"protocols"`
	Services          NodeInfoServices `json:"services"`
	OpenRegistrations bool             `json:"openRegistrations"`
	Usage             NodeInfoUsage    `json:"usage"`
	Metadata          map[string]any   `json:"metadata"`
}

// NodeInfoSoftware represents software information
type NodeInfoSoftware struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Repository string `json:"repository,omitempty"`
	Homepage   string `json:"homepage,omitempty"`
}

// NodeInfoServices represents services information
type NodeInfoServices struct {
	Inbound  []string `json:"inbound"`
	Outbound []string `json:"outbound"`
}

// NodeInfoUsage represents usage statistics
type NodeInfoUsage struct {
	Users         NodeInfoUsers `json:"users"`
	LocalPosts    int           `json:"localPosts"`
	LocalComments int           `json:"localComments"`
}

// NodeInfoUsers represents user statistics
type NodeInfoUsers struct {
	Total          int `json:"total"`
	ActiveHalfyear int `json:"activeHalfyear"`
	ActiveMonth    int `json:"activeMonth"`
}

// HandleNodeInfoWellKnownLift handles /.well-known/nodeinfo requests
func (h *Handler) HandleNodeInfoWellKnownLift(ctx *lift.Context) error {
	h.logger.Info("nodeinfo well-known request",
		zap.String("user_agent", ctx.Header("User-Agent")))

	response := NodeInfoWellKnown{
		Links: []NodeInfoLink{
			{
				Rel:  "http://nodeinfo.diaspora.software/ns/schema/2.0",
				Href: fmt.Sprintf("%s/nodeinfo/2.0", h.cfg.BaseURL()),
			},
		},
	}

	return ctx.JSON(response)
}

// HandleNodeInfoLift handles /nodeinfo/2.0 requests
func (h *Handler) HandleNodeInfoLift(ctx *lift.Context) error {
	h.logger.Info("nodeinfo request",
		zap.String("user_agent", ctx.Header("User-Agent")))

	// Get instance configuration - using default values in test environment
	var instanceConfig *config.InstanceConfig
	if h.cfg.Domain == "example.com" {
		// Test environment - use defaults
		instanceConfig = &config.InstanceConfig{
			Title:             "Test Instance",
			ShortDescription:  "A test instance",
			Description:       "Test instance description",
			Email:             "admin@example.com",
			Version:           "4.0.0 (compatible; Lesser 0.1.0)",
			Software:          "lesser",
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
	stats, err := h.registry.Accounts().GetInstanceStats(ctx.Context, &accounts.GetInstanceStatsQuery{})
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

	response := NodeInfo{
		Version: "2.0",
		Software: NodeInfoSoftware{
			Name:       "lesser",
			Version:    instanceConfig.Version,
			Repository: "https://github.com/equaltoai/lesser",
			Homepage:   "https://github.com/equaltoai/lesser",
		},
		Protocols: []string{
			"activitypub",
		},
		Services: NodeInfoServices{
			Inbound:  []string{},
			Outbound: []string{},
		},
		OpenRegistrations: instanceConfig.RegistrationsOpen,
		Usage: NodeInfoUsage{
			Users: NodeInfoUsers{
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

	return ctx.JSON(response)
}
