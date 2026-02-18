package models

// This file contains helper types for instance-owned configuration patches and effective-resolution outputs.
//
// The persisted record models live in:
// - instance_trust_config.go (SK="TRUST_CONFIG")
// - instance_translation_config.go (SK="TRANSLATION_CONFIG")
// - instance_tips_config.go (SK="TIPS_CONFIG")
// - instance_config.go (SK="AI_CONFIG")

// InstanceTrustConfigPatch is a merge-safe patch for the trust configuration layers.
// Nil fields mean "no change".
type InstanceTrustConfigPatch struct {
	BaseURL              *string
	AttestationsURL      *string
	InstanceKeySecretARN *string
}

// EffectiveTrustConfig contains resolved trust configuration values after applying managed defaults and overrides.
type EffectiveTrustConfig struct {
	TrustBaseURL         string
	AttestationsBaseURL  string
	InstanceKeySecretARN string

	// TrustProxyEnabled indicates whether instance-authenticated trust proxy calls should work.
	TrustProxyEnabled bool

	// PublicAttestationsEnabled indicates whether public JWKS/attestation proxy calls should work.
	PublicAttestationsEnabled bool
}

// InstanceTranslationConfigPatch is a merge-safe patch for the translation configuration layers.
type InstanceTranslationConfigPatch struct {
	Enabled *bool
}

// InstanceTipsConfigPatch is a merge-safe patch for the tips configuration layers.
type InstanceTipsConfigPatch struct {
	Enabled         *bool
	ChainID         *int
	ContractAddress *string
}

// EffectiveTipsConfig contains resolved tips configuration values after applying managed defaults and overrides.
type EffectiveTipsConfig struct {
	Enabled         bool
	ChainID         int
	ContractAddress string
}

// AIInstanceConfigPatch is a merge-safe patch for the AI configuration layers.
type AIInstanceConfigPatch struct {
	AIEnabled            *bool
	ModerationEnabled    *bool
	NSFWDetectionEnabled *bool
	SpamDetectionEnabled *bool
	PIIDetectionEnabled  *bool
	AIContentDetection   *bool
}

// EffectiveAIInstanceConfig contains resolved AI configuration values after applying managed defaults and overrides.
type EffectiveAIInstanceConfig struct {
	AIEnabled            bool
	ModerationEnabled    bool
	NSFWDetectionEnabled bool
	SpamDetectionEnabled bool
	PIIDetectionEnabled  bool
	AIContentDetection   bool
}
