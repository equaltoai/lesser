package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"fmt"
	"io"
)

const (
	// MaxUntrustedHTTPResponseBodyBytes is the default maximum number of bytes to read from
	// untrusted HTTP responses when the body is only needed for logging/debugging.
	//
	// This is intentionally far smaller than MaxRequestSize: untrusted servers may return
	// arbitrarily large bodies, and we should never allow them to amplify memory/cost.
	MaxUntrustedHTTPResponseBodyBytes = 64 * 1024 // 64KB

	// TruncatedBodyMarker is appended to snippets when the body exceeded the cap.
	TruncatedBodyMarker = "…[truncated]"
)

// ReadUntrustedHTTPResponseBody reads up to maxBytes from r and reports whether the response
// body was truncated. It never allocates more than maxBytes+1.
func ReadUntrustedHTTPResponseBody(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		maxBytes = MaxUntrustedHTTPResponseBodyBytes
	}

	limited := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, fmt.Errorf("read response body: %w", err)
	}

	if int64(len(data)) > maxBytes {
		return data[:maxBytes], true, nil
	}

	return data, false, nil
}

// FormatUntrustedHTTPBodySnippet returns a stable string snippet for logs and stored error fields.
func FormatUntrustedHTTPBodySnippet(body []byte, truncated bool) string {
	if truncated {
		return string(body) + TruncatedBodyMarker
	}
	return string(body)
}
