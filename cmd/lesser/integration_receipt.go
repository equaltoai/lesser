package main

import (
	"os"
	"strconv"
	"strings"
)

func resolveIntegrationReceipt(args upArgs) *integrationReceipt {
	out := &integrationReceipt{}

	out.LesserHostURL = strings.TrimRight(strings.TrimSpace(firstNonEmpty(args.LesserHostURL, os.Getenv("LESSER_HOST_URL"))), "/")
	out.LesserHostAttestationsURL = strings.TrimRight(strings.TrimSpace(firstNonEmpty(args.LesserHostAttestationsURL, os.Getenv("LESSER_HOST_ATTESTATIONS_URL"))), "/")
	out.LesserHostInstanceKeyARN = strings.TrimSpace(firstNonEmpty(args.LesserHostInstanceKeyARN, os.Getenv("LESSER_HOST_INSTANCE_KEY_ARN")))
	out.BodyEnabled = resolveOptionalBoolEnv("BODY_ENABLED", true)

	if args.TranslationEnabled != nil {
		out.TranslationEnabled = args.TranslationEnabled
	} else if raw := strings.TrimSpace(os.Getenv("TRANSLATION_ENABLED")); raw != "" {
		enabled := raw == flagTrue || raw == "1" || raw == flagYes
		out.TranslationEnabled = &enabled
	}

	if args.TipEnabled != nil {
		out.TipEnabled = args.TipEnabled
	} else if raw := strings.TrimSpace(os.Getenv("TIP_ENABLED")); raw != "" {
		enabled := raw == flagTrue || raw == "1" || raw == flagYes
		out.TipEnabled = &enabled
	}

	if args.TipChainID != nil {
		out.TipChainID = args.TipChainID
	} else if raw := strings.TrimSpace(os.Getenv("TIP_CHAIN_ID")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			out.TipChainID = &v
		}
	}

	out.TipContractAddress = strings.TrimSpace(firstNonEmpty(args.TipContractAddress, os.Getenv("TIP_CONTRACT_ADDRESS")))

	out.AIEnabled = args.AIEnabled
	out.AIModerationEnabled = args.AIModerationEnabled
	out.AINsfwDetectionEnabled = args.AINsfwDetectionEnabled
	out.AISpamDetectionEnabled = args.AISpamDetectionEnabled
	out.AIPiiDetectionEnabled = args.AIPiiDetectionEnabled
	out.AIContentDetectionEnabled = args.AIContentDetectionEnabled

	if out.LesserHostURL == "" &&
		out.LesserHostAttestationsURL == "" &&
		out.LesserHostInstanceKeyARN == "" &&
		out.BodyEnabled == nil &&
		out.TranslationEnabled == nil &&
		out.TipEnabled == nil &&
		out.TipChainID == nil &&
		strings.TrimSpace(out.TipContractAddress) == "" &&
		out.AIEnabled == nil &&
		out.AIModerationEnabled == nil &&
		out.AINsfwDetectionEnabled == nil &&
		out.AISpamDetectionEnabled == nil &&
		out.AIPiiDetectionEnabled == nil &&
		out.AIContentDetectionEnabled == nil {
		return nil
	}
	return out
}

func resolveOptionalBoolEnv(key string, defaultValue bool) *bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		value := defaultValue
		return &value
	}

	enabled := raw == flagTrue || raw == "1" || raw == flagYes
	return &enabled
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
