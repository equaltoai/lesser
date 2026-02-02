package agents

// Capabilities describe what an agent account is permitted to do at a high level.
//
// This is intentionally coarse-grained; operational enforcement (rate limits, quarantine,
// circuit breakers, and per-endpoint policy) is handled elsewhere.
type Capabilities struct {
	CanPost   bool `json:"can_post"`
	CanReply  bool `json:"can_reply"`
	CanBoost  bool `json:"can_boost"`
	CanFollow bool `json:"can_follow"`
	CanDM     bool `json:"can_dm"`

	RestrictedDomains []string `json:"restricted_domains,omitempty"`
	MaxPostsPerHour   int      `json:"max_posts_per_hour"`
	RequiresApproval  bool     `json:"requires_approval"`
}
