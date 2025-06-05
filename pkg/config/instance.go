package config

import (
	"os"
	"strconv"
)

// InstanceConfig holds static instance configuration
type InstanceConfig struct {
	// From environment variables
	Title            string
	ShortDescription string
	Description      string
	Email            string

	// Static configuration
	Version        string
	Software       string
	MaxStatusChars int
	MaxMediaSize   int64
	MaxVideoSize   int64
	Languages      []string

	// Feature flags
	RegistrationsOpen bool
	ApprovalRequired  bool
	InvitesEnabled    bool
	FederationEnabled bool
}

// GetInstanceConfig returns the instance configuration
func GetInstanceConfig() *InstanceConfig {
	return &InstanceConfig{
		// From environment
		Title:            getInstanceEnv("INSTANCE_TITLE", "Lesser Instance"),
		ShortDescription: getInstanceEnv("INSTANCE_SHORT_DESC", "A personal ActivityPub server"),
		Description:      getInstanceEnv("INSTANCE_DESCRIPTION", "A lightweight, serverless ActivityPub implementation"),
		Email:            getInstanceEnv("INSTANCE_ADMIN_EMAIL", "admin@"+Get().Domain),

		// Static defaults
		Version:        "4.0.0 (compatible; Lesser 0.1.0)",
		Software:       "lesser",
		MaxStatusChars: getInstanceEnvInt("MAX_STATUS_CHARS", 5000),
		MaxMediaSize:   getInstanceEnvInt64("MAX_MEDIA_SIZE", 10*1024*1024), // 10MB
		MaxVideoSize:   getInstanceEnvInt64("MAX_VIDEO_SIZE", 40*1024*1024), // 40MB
		Languages:      []string{"en"},                                      // TODO: Make configurable

		// Feature flags
		RegistrationsOpen: getInstanceEnvBool("REGISTRATIONS_OPEN", false),
		ApprovalRequired:  getInstanceEnvBool("APPROVAL_REQUIRED", true),
		InvitesEnabled:    getInstanceEnvBool("INVITES_ENABLED", false),
		FederationEnabled: getInstanceEnvBool("FEDERATION_ENABLED", true),
	}
}

func getInstanceEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getInstanceEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getInstanceEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

func getInstanceEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
