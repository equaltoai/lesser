package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/config"
)

const (
	remoteStatusIDPrefix = "remote_"
	schemeHTTP           = "http"
	schemeHTTPS          = "https"
)

// CanonicalStatusID normalizes local and remote status identifiers into one
// storage-safe contract. Local status URLs collapse to their path identifier,
// while remote status URLs hash to a deterministic remote-prefixed ID so
// different domains cannot collide on the same final path segment.
func CanonicalStatusID(raw string) string {
	return CanonicalStatusIDForDomain(raw, config.Get().Domain)
}

// StatusLookupCandidates returns the meaningful status identifiers that should be
// attempted for a direct status read against the current local domain.
func StatusLookupCandidates(raw string) []string {
	return StatusLookupCandidatesForDomain(raw, config.Get().Domain)
}

// CanonicalActivityPubObjectIDForStatus returns the ActivityPub object ID that
// should be used when a federated interaction refers to a status. This is
// intentionally distinct from StatusID: remote statuses use a local remote_*
// StatusID for storage and counters, while their ActivityPub object identity
// remains the source instance's Note ID.
func CanonicalActivityPubObjectIDForStatus(status *Status, localDomain string) string {
	if status == nil {
		return ""
	}

	if status.Note != nil {
		if noteID := strings.TrimSpace(status.Note.ID); noteID != "" {
			return noteID
		}
	}

	for _, rawURL := range status.URLs {
		rawURL = strings.TrimSpace(rawURL)
		if isHTTPStatusURL(rawURL) {
			return rawURL
		}
	}

	if statusAppearsRemote(status, localDomain) {
		return ""
	}

	return localActivityPubObjectIDForStatus(status, localDomain)
}

// StatusLookupCandidatesForDomain returns the meaningful status identifiers that
// should be attempted for a direct status read against the supplied local domain.
func StatusLookupCandidatesForDomain(raw string, localDomain string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	candidates := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	appendCandidate := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	if _, parsed, ok := normalizeStatusIdentifierURL(raw); ok {
		pathID := localStatusIDFromNormalizedURL(parsed)
		canonicalID := CanonicalStatusIDForDomain(raw, localDomain)
		if isLocalStatusIdentifierHostForDomain(parsed.Hostname(), localDomain) {
			appendCandidate(pathID)
			appendCandidate(canonicalID)
			return candidates
		}

		appendCandidate(canonicalID)
		appendCandidate(pathID)
		return candidates
	}

	appendCandidate(raw)
	return candidates
}

func isHTTPStatusURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return false
	}
	return (parsed.Scheme == schemeHTTP || parsed.Scheme == schemeHTTPS) && parsed.Host != ""
}

func statusAppearsRemote(status *Status, localDomain string) bool {
	if status == nil {
		return false
	}
	if IsCanonicalRemoteStatusID(status.StatusID) {
		return true
	}
	if strings.Contains(strings.TrimSpace(status.AuthorUsername), "@") {
		return true
	}

	actorID := strings.TrimSpace(status.AuthorID)
	if actorID == "" && status.Note != nil {
		actorID = strings.TrimSpace(status.Note.AttributedTo)
	}
	if actorID == "" {
		return false
	}

	_, parsed, ok := normalizeStatusIdentifierURL(actorID)
	if !ok {
		return false
	}
	return !isLocalStatusIdentifierHostForDomain(parsed.Hostname(), localDomain)
}

func localActivityPubObjectIDForStatus(status *Status, localDomain string) string {
	if status == nil {
		return ""
	}

	statusID := strings.TrimSpace(status.StatusID)
	if statusID == "" {
		return ""
	}

	domain := normalizedLocalDomain(localDomain)
	if domain == "" {
		return ""
	}

	author := strings.TrimSpace(status.AuthorUsername)
	if author == "" {
		author = usernameFromActorIDForStatusIdentity(status.AuthorID)
	}
	if author == "" && status.Note != nil {
		author = usernameFromActorIDForStatusIdentity(status.Note.AttributedTo)
	}
	if author == "" || strings.Contains(author, "@") {
		return ""
	}

	return fmt.Sprintf("https://%s/users/%s/statuses/%s", domain, author, statusID)
}

func usernameFromActorIDForStatusIdentity(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}

	parsed, err := url.Parse(actorID)
	if err == nil && parsed != nil && parsed.Path != "" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}

	trimmed := strings.TrimSuffix(actorID, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	return strings.TrimSpace(parts[len(parts)-1])
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
	case parsed.Scheme == schemeHTTPS && port == "443":
		port = ""
	case parsed.Scheme == schemeHTTP && port == "80":
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

	localDomain = normalizedLocalDomain(localDomain)
	if localDomain == "" {
		return false
	}

	return host == localDomain
}

func normalizedLocalDomain(localDomain string) string {
	localDomain = strings.ToLower(strings.TrimSpace(localDomain))
	localDomain = strings.TrimPrefix(strings.TrimPrefix(localDomain, "https://"), "http://")
	if idx := strings.Index(localDomain, "/"); idx >= 0 {
		localDomain = localDomain[:idx]
	}
	if idx := strings.Index(localDomain, ":"); idx >= 0 {
		localDomain = localDomain[:idx]
	}
	return localDomain
}
