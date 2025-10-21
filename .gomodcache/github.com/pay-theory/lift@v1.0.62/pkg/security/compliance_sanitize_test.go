package security

import (
	"strings"
	"testing"
)

// test-only helpers mirroring sanitizer behavior
func sanitizeHeadersTest(headers map[string][]string) map[string]string {
	sanitized := make(map[string]string)
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "auth") || strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "cookie") || strings.Contains(lowerKey, "key") {
			sanitized[key] = "[REDUCTED]"
		} else {
			sanitized[key] = values[0]
		}
	}
	return sanitized
}

func sanitizeQueryParamsTest(params map[string][]string) map[string]string {
	sanitized := make(map[string]string)
	for key, values := range params {
		if len(values) == 0 {
			continue
		}
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "key") {
			sanitized[key] = "[REDUCTED]"
		} else {
			sanitized[key] = values[0]
		}
	}
	return sanitized
}

func TestComplianceSanitizeHelpers_CompileUse(t *testing.T) {
	t.Parallel()
	headers := map[string][]string{"Authorization": {"Bearer abc"}}
	params := map[string][]string{"token": {"abc"}}
	_ = sanitizeHeadersTest(headers)
	_ = sanitizeQueryParamsTest(params)
}
