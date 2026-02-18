package models

// This file contains helper types for instance-owned configuration patches and effective-resolution outputs.
//
// The persisted record models live in:
// - instance_trust_config.go (SK="TRUST_CONFIG")
// - instance_translation_config.go (SK="TRANSLATION_CONFIG")
// - instance_tips_config.go (SK="TIPS_CONFIG")
// - instance_config.go (SK="AI_CONFIG")

type InstanceTrustConfigPatch struct {
	BaseURL              *string
	AttestationsURL      *string
	InstanceKeySecretARN *string
}

type EffectiveTrustConfig struct {
	TrustBaseURL         string
	AttestationsBaseURL  string
	InstanceKeySecretARN string

	// TrustProxyEnabled indicates whether instance-authenticated trust proxy calls should work.
	TrustProxyEnabled bool

	// PublicAttestationsEnabled indicates whether public JWKS/attestation proxy calls should work.
	PublicAttestationsEnabled bool
}

type InstanceTranslationConfigPatch struct {
	Enabled *bool
}

type InstanceTipsConfigPatch struct {
	Enabled         *bool
	ChainID         *int
	ContractAddress *string
}

type EffectiveTipsConfig struct {
	Enabled         bool
	ChainID         int
	ContractAddress string
}

type AIInstanceConfigPatch struct {
	AIEnabled            *bool
	ModerationEnabled    *bool
	NSFWDetectionEnabled *bool
	SpamDetectionEnabled *bool
	PIIDetectionEnabled  *bool
	AIContentDetection   *bool
}

type EffectiveAIInstanceConfig struct {
	AIEnabled            bool
	ModerationEnabled    bool
	NSFWDetectionEnabled bool
	SpamDetectionEnabled bool
	PIIDetectionEnabled  bool
	AIContentDetection   bool
}
