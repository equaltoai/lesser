package models

import "strings"

const (
	instanceConfigPK = "INSTANCE#CONFIG"

	// SKTrustConfig is the sort key for the trust configuration record (PK="INSTANCE#CONFIG").
	SKTrustConfig = "TRUST_CONFIG"

	// SKTranslationConfig is the sort key for the translation configuration record (PK="INSTANCE#CONFIG").
	SKTranslationConfig = "TRANSLATION_CONFIG"

	// SKTipsConfig is the sort key for the tips configuration record (PK="INSTANCE#CONFIG").
	SKTipsConfig = "TIPS_CONFIG"

	// SKAIConfig is the sort key for the AI configuration record (PK="INSTANCE#CONFIG").
	SKAIConfig = "AI_CONFIG"

	// SKWellKnownLesserSoulAgent is the sort key for the soul HTTPS proof record (PK="INSTANCE#CONFIG").
	SKWellKnownLesserSoulAgent = "WELL_KNOWN_LESSER_SOUL_AGENT"

	// SKSoulENSChannelPrefix is the sort-key prefix for per-agent soul ENS channel config records.
	SKSoulENSChannelPrefix = "SOUL_ENS_CHANNEL#"

	// SKSoulBodyBindingPrefix is the sort-key prefix for per-agent soul/body binding records.
	SKSoulBodyBindingPrefix = "SOUL_BODY_BINDING#"

	// SKSoulBodyBindingUsername is the sort key for username reverse-index records.
	SKSoulBodyBindingUsername = "SOUL_BODY_BINDING"
)

// SoulENSChannelSortKey returns the sort key for an instance-owned soul ENS channel config record.
func SoulENSChannelSortKey(agentID string) string {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return ""
	}
	return SKSoulENSChannelPrefix + agentID
}

// SoulBodyBindingSortKey returns the sort key for an instance-owned soul/body binding record.
func SoulBodyBindingSortKey(agentID string) string {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return ""
	}
	return SKSoulBodyBindingPrefix + agentID
}

// SoulBodyBindingUsernamePartitionKey returns the partition key for username reverse-index records.
func SoulBodyBindingUsernamePartitionKey(username string) string {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return ""
	}
	return "SOUL_BODY_BINDING_USERNAME#" + username
}
