package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Redacted_ReplacesAllSecretFields(t *testing.T) {
	secretValue := "super-secret-do-not-leak"
	cfg := &Config{
		JWTSecret:                 secretValue,
		ReputationPrivateKey:      secretValue,
		PrivacyMasterKey:          secretValue,
		InstanceAPIKey:            secretValue,
		LesserHostInstanceKey:     secretValue,
		CloudFrontPrivateKeyPath:  secretValue,
		DynamoDBEncryptionKey:     secretValue,
		ActorPrivateKeyEncryption: secretValue,
		// Webhook URLs carry embedded tokens and must be redacted
		AlertWebhookURL:       "https://example-webhook.test/alert/abc123-token-xyz",
		BudgetAlertWebhookURL: "https://discord.com/api/webhooks/1234567890/abc-def_ghi-jkl_mno",
		// Non-secret fields that should be preserved
		Domain:       "example.com",
		InstanceName: "Test Instance",
		Region:       "us-east-1",
		Stage:        "dev",
		JWTSecretARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:jwt",
	}

	redacted := cfg.Redacted()
	require.NotNil(t, redacted)

	// Secret fields must be redacted
	assert.Equal(t, RedactedSecretSentinel, redacted.JWTSecret)
	assert.Equal(t, RedactedSecretSentinel, redacted.ReputationPrivateKey)
	assert.Equal(t, RedactedSecretSentinel, redacted.PrivacyMasterKey)
	assert.Equal(t, RedactedSecretSentinel, redacted.InstanceAPIKey)
	assert.Equal(t, RedactedSecretSentinel, redacted.LesserHostInstanceKey)
	assert.Equal(t, RedactedSecretSentinel, redacted.CloudFrontPrivateKeyPath)
	assert.Equal(t, RedactedSecretSentinel, redacted.DynamoDBEncryptionKey)
	assert.Equal(t, RedactedSecretSentinel, redacted.ActorPrivateKeyEncryption)
	assert.Equal(t, RedactedSecretSentinel, redacted.AlertWebhookURL)
	assert.Equal(t, RedactedSecretSentinel, redacted.BudgetAlertWebhookURL)

	// Non-secret fields must be preserved
	assert.Equal(t, "example.com", redacted.Domain)
	assert.Equal(t, "Test Instance", redacted.InstanceName)
	assert.Equal(t, "us-east-1", redacted.Region)
	assert.Equal(t, "dev", redacted.Stage)

	// ARN pointers must be preserved (not secret values themselves)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:jwt", redacted.JWTSecretARN)

	// Verify redaction is a shallow copy — original must not be mutated
	assert.Equal(t, secretValue, cfg.JWTSecret, "original config must not be mutated by Redacted()")
}

func TestConfig_Redacted_NilReceiver(t *testing.T) {
	var cfg *Config
	assert.Nil(t, cfg.Redacted())
}

func TestConfig_Redacted_EmptyStringsPreserved(t *testing.T) {
	cfg := &Config{
		JWTSecret:                 "",
		ReputationPrivateKey:      "",
		PrivacyMasterKey:          "",
		InstanceAPIKey:            "",
		LesserHostInstanceKey:     "",
		CloudFrontPrivateKeyPath:  "",
		DynamoDBEncryptionKey:     "",
		ActorPrivateKeyEncryption: "",
		AlertWebhookURL:           "",
		BudgetAlertWebhookURL:     "",
		Domain:                    "",
	}

	redacted := cfg.Redacted()
	require.NotNil(t, redacted)

	// Empty secret fields still become RedactedSecretSentinel
	assert.Equal(t, RedactedSecretSentinel, redacted.JWTSecret)
	assert.Equal(t, RedactedSecretSentinel, redacted.PrivacyMasterKey)

	// Empty non-secret fields remain empty
	assert.Equal(t, "", redacted.Domain)
}
