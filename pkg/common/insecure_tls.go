package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"os"
	"strconv"
	"strings"
)

// InsecureTLSOverrideEnvVar enables explicitly allowing insecure TLS in narrowly scoped, debug-only scenarios.
//
// Insecure TLS means certificate verification may be disabled (e.g., tls.Config.InsecureSkipVerify).
const InsecureTLSOverrideEnvVar = "LESSER_ALLOW_INSECURE_TLS"

// InsecureTLSOverrideEnabled reports whether the runtime explicitly allows disabling TLS verification.
//
// This is a defense-in-depth guardrail: callers must opt-in via an environment variable before any
// "insecure TLS" code path can be activated.
func InsecureTLSOverrideEnabled() bool {
	value := strings.TrimSpace(os.Getenv(InsecureTLSOverrideEnvVar))
	enabled, err := strconv.ParseBool(value)
	return err == nil && enabled
}
