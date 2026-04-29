package stacks

import (
	"os"
	"strings"
	"testing"
)

func TestClientSSRPlaceholderHardeningAsset(t *testing.T) {
	data, err := os.ReadFile(clientSSRHostAssetPath() + "/index.mjs")
	if err != nil {
		t.Fatalf("read client SSR host asset: %v", err)
	}
	code := string(data)

	requireContainsAll(t, code, []string{
		"function publicOrigin(event)",
		"headerValue(event, \"x-lesser-forwarded-host\")",
		"function sanitizeHost(value)",
		"escapeHtml(origin)",
		"\"content-security-policy\"",
		"\"x-content-type-options\": \"nosniff\"",
		"\"x-frame-options\": \"DENY\"",
		"\"permissions-policy\"",
		".replaceAll('\"', \"&quot;\")",
		".replaceAll(\"'\", \"&#39;\")",
	})

	if strings.Contains(code, "href=\"${origin}") {
		t.Fatalf("placeholder must not interpolate unescaped origin into href attributes")
	}
}
