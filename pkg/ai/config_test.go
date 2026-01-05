package ai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultAIConfig(t *testing.T) {
	cfg := DefaultAIConfig()
	require.NotNil(t, cfg)
	require.True(t, cfg.EnablePIIDetection)
	require.True(t, cfg.EnableAIDetection)
	require.True(t, cfg.EnableImageAnalysis)
	require.Equal(t, "lesser-media-analysis", cfg.S3Bucket)
	require.Equal(t, "anthropic.claude-v2", cfg.BedrockModelID)
}

func TestGetThresholds_DefaultsToNote(t *testing.T) {
	note := GetThresholds("note")
	unknown := GetThresholds("unknown-type")
	require.Equal(t, note, unknown)
}

func TestGetSignalWeight_DefaultForUnknown(t *testing.T) {
	require.Equal(t, 0.1, GetSignalWeight("unknown-signal"))
	require.Equal(t, 0.3, GetSignalWeight("toxicity"))
}
