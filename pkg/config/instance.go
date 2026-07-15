package config

import (
	"fmt"
	"strings"
)

const mastodonCompatibilityVersion = "4.0.0"

// InstanceConfig holds static instance configuration
type InstanceConfig struct {
	// From environment variables
	Title            string
	ShortDescription string
	Description      string
	Email            string

	// Static configuration
	Version         string
	Software        string
	SoftwareVersion string
	MaxStatusChars  int
	MaxMediaSize    int64
	MaxVideoSize    int64
	Languages       []string

	// Feature flags
	RegistrationsOpen bool
	ApprovalRequired  bool
	InvitesEnabled    bool
	FederationEnabled bool
}

// GetInstanceConfig returns the instance configuration
func GetInstanceConfig() *InstanceConfig {
	cfg := Get() // Get centralized config
	lesserVersion := NormalizeLesserVersion(cfg.Version)

	return &InstanceConfig{
		// From centralized config
		Title:            cfg.InstanceTitle,
		ShortDescription: cfg.InstanceShortDesc,
		Description:      cfg.InstanceDescription,
		Email:            cfg.InstanceAdminEmail,

		// Static defaults and public version provenance.
		Version:         MastodonCompatibleVersion(lesserVersion),
		Software:        "lesser",
		SoftwareVersion: lesserVersion,
		MaxStatusChars:  int(cfg.MaxStatusChars),
		MaxMediaSize:    cfg.MaxMediaSize,
		MaxVideoSize:    cfg.MaxVideoSize,
		Languages:       cfg.InstanceLanguages,

		// Feature flags from centralized config
		RegistrationsOpen: cfg.RegistrationsOpen,
		ApprovalRequired:  cfg.ApprovalRequired,
		InvitesEnabled:    cfg.InvitesEnabled,
		FederationEnabled: cfg.FederationEnabled,
	}
}

// NormalizeLesserVersion returns the release/build provenance string that
// public discovery surfaces should advertise for Lesser itself.
func NormalizeLesserVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "dev"
	}
	return value
}

// MastodonCompatibleVersion returns the public Mastodon REST compatibility
// string while preserving accurate Lesser release provenance.
func MastodonCompatibleVersion(lesserVersion string) string {
	return fmt.Sprintf("%s (compatible; Lesser %s)", mastodonCompatibilityVersion, NormalizeLesserVersion(lesserVersion))
}
