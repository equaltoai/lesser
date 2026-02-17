package main

import (
	"os"
	"strings"
)

func resolveIntegrationReceipt(args upArgs) *integrationReceipt {
	out := &integrationReceipt{}

	out.LesserHostURL = strings.TrimRight(strings.TrimSpace(firstNonEmpty(args.LesserHostURL, os.Getenv("LESSER_HOST_URL"))), "/")
	out.LesserHostAttestationsURL = strings.TrimRight(strings.TrimSpace(firstNonEmpty(args.LesserHostAttestationsURL, os.Getenv("LESSER_HOST_ATTESTATIONS_URL"))), "/")
	out.LesserHostInstanceKeyARN = strings.TrimSpace(firstNonEmpty(args.LesserHostInstanceKeyARN, os.Getenv("LESSER_HOST_INSTANCE_KEY_ARN")))

	if args.TranslationEnabled != nil {
		out.TranslationEnabled = args.TranslationEnabled
	} else if raw := strings.TrimSpace(os.Getenv("TRANSLATION_ENABLED")); raw != "" {
		enabled := raw == "true" || raw == "1" || raw == "yes"
		out.TranslationEnabled = &enabled
	}

	if out.LesserHostURL == "" && out.LesserHostAttestationsURL == "" && out.LesserHostInstanceKeyARN == "" && out.TranslationEnabled == nil {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
