package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveIntegrationReceipt_DefaultsBodyEnabledWhenOtherwiseEmpty(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", "")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", "")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", "")
	t.Setenv("BODY_ENABLED", "")
	t.Setenv("TRANSLATION_ENABLED", "")
	t.Setenv("TIP_ENABLED", "")
	t.Setenv("TIP_CHAIN_ID", "")
	t.Setenv("TIP_CONTRACT_ADDRESS", "")

	out := resolveIntegrationReceipt(upArgs{})
	require.NotNil(t, out)
	require.NotNil(t, out.BodyEnabled)
	require.True(t, *out.BodyEnabled)
}

func TestResolveIntegrationReceipt_ResolvesArgsAndEnv(t *testing.T) {
	t.Setenv("LESSER_HOST_URL", " https://env.example/ ")
	t.Setenv("LESSER_HOST_ATTESTATIONS_URL", " https://env-attest.example/ ")
	t.Setenv("LESSER_HOST_INSTANCE_KEY_ARN", " arn:aws:secretsmanager:us-east-1:123:secret:abc ")
	t.Setenv("BODY_ENABLED", "false")
	t.Setenv("TRANSLATION_ENABLED", "yes")
	t.Setenv("TIP_ENABLED", "1")
	t.Setenv("TIP_CHAIN_ID", "10")
	t.Setenv("TIP_CONTRACT_ADDRESS", " 0xenv ")

	translationEnabled := false
	args := upArgs{
		LesserHostURL:            " https://args.example/ ",
		TranslationEnabled:       &translationEnabled,
		TipContractAddress:       " 0xargs ",
		LesserHostInstanceKeyARN: "",
	}

	out := resolveIntegrationReceipt(args)
	require.NotNil(t, out)
	require.Equal(t, "https://args.example", out.LesserHostURL)
	require.Equal(t, "https://env-attest.example", out.LesserHostAttestationsURL)
	require.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:abc", out.LesserHostInstanceKeyARN)
	require.NotNil(t, out.BodyEnabled)
	require.False(t, *out.BodyEnabled)

	require.NotNil(t, out.TranslationEnabled)
	require.False(t, *out.TranslationEnabled)

	require.NotNil(t, out.TipEnabled)
	require.True(t, *out.TipEnabled)

	require.NotNil(t, out.TipChainID)
	require.Equal(t, 10, *out.TipChainID)

	require.Equal(t, "0xargs", out.TipContractAddress)
}

func TestResolveIntegrationReceipt_IgnoresBadChainID(t *testing.T) {
	t.Setenv("TIP_ENABLED", "true")
	t.Setenv("TIP_CHAIN_ID", "bad")

	out := resolveIntegrationReceipt(upArgs{})
	require.NotNil(t, out)
	require.NotNil(t, out.TipEnabled)
	require.Nil(t, out.TipChainID)
}

func TestResolveIntegrationReceipt_UsesArgsPointersAndEnvParsing(t *testing.T) {
	t.Setenv("TRANSLATION_ENABLED", "1")
	t.Setenv("TIP_ENABLED", "true")

	tipEnabled := false
	chainID := 1
	out := resolveIntegrationReceipt(upArgs{
		TipEnabled:         &tipEnabled,
		TipChainID:         &chainID,
		TipContractAddress: "",
	})
	require.NotNil(t, out)
	require.NotNil(t, out.TranslationEnabled)
	require.True(t, *out.TranslationEnabled)
	require.NotNil(t, out.TipEnabled)
	require.False(t, *out.TipEnabled)
	require.NotNil(t, out.TipChainID)
	require.Equal(t, 1, *out.TipChainID)
}

func TestFirstNonEmpty_TrimsWhitespace(t *testing.T) {
	require.Equal(t, " x ", firstNonEmpty("  ", " x ", "y"))
	require.Equal(t, "", firstNonEmpty("", "   "))
}
