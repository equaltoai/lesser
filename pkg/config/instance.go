package config

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
	cfg := Get() // Get centralized config

	return &InstanceConfig{
		// From centralized config
		Title:            cfg.InstanceTitle,
		ShortDescription: cfg.InstanceShortDesc,
		Description:      cfg.InstanceDescription,
		Email:            cfg.InstanceAdminEmail,

		// Static defaults
		Version:        "4.0.0 (compatible; Lesser 0.1.0)",
		Software:       "lesser",
		MaxStatusChars: int(cfg.MaxStatusChars),
		MaxMediaSize:   cfg.MaxMediaSize,
		MaxVideoSize:   cfg.MaxVideoSize,
		Languages:      cfg.InstanceLanguages,

		// Feature flags from centralized config
		RegistrationsOpen: cfg.RegistrationsOpen,
		ApprovalRequired:  cfg.ApprovalRequired,
		InvitesEnabled:    cfg.InvitesEnabled,
		FederationEnabled: cfg.FederationEnabled,
	}
}
