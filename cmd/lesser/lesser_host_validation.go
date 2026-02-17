package main

import (
	"fmt"
	"net/url"
	"strings"
)

func rejectLambdaFunctionURLHost(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	if u, err := url.Parse(raw); err == nil {
		if host := strings.ToLower(strings.TrimSpace(u.Hostname())); isLambdaFunctionURLHost(host) {
			return fmt.Errorf("invalid LESSER_HOST_URL %q: Lambda Function URL hosts are not supported; use a domain-only lesser.host endpoint", raw)
		}
	}

	lower := strings.ToLower(raw)
	if strings.Contains(lower, ".lambda-url.") && strings.Contains(lower, ".on.aws") {
		return fmt.Errorf("invalid LESSER_HOST_URL %q: Lambda Function URL hosts are not supported; use a domain-only lesser.host endpoint", raw)
	}

	return nil
}

func isLambdaFunctionURLHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.Contains(host, ".lambda-url.") && strings.HasSuffix(host, ".on.aws")
}
