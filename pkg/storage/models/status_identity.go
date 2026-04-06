package models

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/config"
)

const remoteStatusIDPrefix = "remote_"

// CanonicalStatusID normalizes local and remote status identifiers into one
// storage-safe contract. Local status URLs collapse to their path identifier,
// while remote status URLs hash to a deterministic remote-prefixed ID so
// different domains cannot collide on the same final path segment.
func CanonicalStatusID(raw string) string {
	return CanonicalStatusIDForDomain(raw, config.Get().Domain)
}

// CanonicalStatusIDForDomain normalizes a status identifier against the
// provided local domain instead of the global process config.
func CanonicalStatusIDForDomain(raw string, localDomain string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if IsCanonicalRemoteStatusID(raw) {
		return raw
	}

	normalizedURL, parsed, ok := normalizeStatusIdentifierURL(raw)
	if !ok {
		return raw
	}

	if isLocalStatusIdentifierHostForDomain(parsed.Hostname(), localDomain) {
		return localStatusIDFromNormalizedURL(parsed)
	}

	sum := sha256.Sum256([]byte(normalizedURL))
	return remoteStatusIDPrefix + hex.EncodeToString(sum[:])
}

// IsCanonicalRemoteStatusID reports whether the identifier is already using the
// canonical remote status identity contract.
func IsCanonicalRemoteStatusID(statusID string) bool {
	statusID = strings.TrimSpace(statusID)
	if !strings.HasPrefix(statusID, remoteStatusIDPrefix) {
		return false
	}

	hexPart := strings.TrimPrefix(statusID, remoteStatusIDPrefix)
	if len(hexPart) != sha256.Size*2 {
		return false
	}

	for _, ch := range hexPart {
		switch {
		case ch >= '0' && ch <= '9':
		case ch >= 'a' && ch <= 'f':
		default:
			return false
		}
	}

	return true
}

func normalizeStatusIdentifierURL(raw string) (string, *url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", nil, false
	}

	parsed.Fragment = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "", nil, false
	}

	port := parsed.Port()
	switch {
	case parsed.Scheme == "https" && port == "443":
		port = ""
	case parsed.Scheme == "http" && port == "80":
		port = ""
	}

	if port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	} else {
		parsed.Host = host
	}

	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	parsed.Path = path
	parsed.RawPath = ""

	return parsed.String(), parsed, true
}

func localStatusIDFromNormalizedURL(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}

	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}

	parts := strings.Split(path, "/")
	return strings.TrimSpace(parts[len(parts)-1])
}

func isLocalStatusIdentifierHostForDomain(host string, localDomain string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}

	localDomain = strings.ToLower(strings.TrimSpace(localDomain))
	localDomain = strings.TrimPrefix(strings.TrimPrefix(localDomain, "https://"), "http://")
	if idx := strings.Index(localDomain, "/"); idx >= 0 {
		localDomain = localDomain[:idx]
	}
	if idx := strings.Index(localDomain, ":"); idx >= 0 {
		localDomain = localDomain[:idx]
	}
	if localDomain == "" {
		return false
	}

	return host == localDomain
}
