package repositories

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/common"
)

func canonicalNumericMappingUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func normalizedNumericIDMappingValues(username, actorID string) (numericID, canonicalUsername, canonicalActorID string) {
	canonicalUsername = canonicalNumericMappingUsername(username)
	if canonicalUsername == "" {
		return "", "", strings.TrimSpace(actorID)
	}

	return common.GenerateNumericID(canonicalUsername), canonicalUsername, normalizeCanonicalActorReference(actorID, canonicalUsername)
}

func normalizeCanonicalActorReference(value, username string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	canonical := canonicalNumericMappingUsername(username)
	if canonical == "" {
		return trimmed
	}

	trimmed = strings.TrimRight(trimmed, "/")
	lowerTrimmed := strings.ToLower(trimmed)

	if idx := strings.LastIndex(lowerTrimmed, "/users/"); idx >= 0 {
		suffix := trimmed[idx+len("/users/"):]
		if suffix != "" && !strings.Contains(suffix, "/") {
			return trimmed[:idx+len("/users/")] + canonical
		}
	}

	if idx := strings.LastIndex(lowerTrimmed, "/@"); idx >= 0 {
		suffix := trimmed[idx+len("/@"):]
		if suffix != "" && !strings.Contains(suffix, "/") {
			return trimmed[:idx+len("/@")] + canonical
		}
	}

	return trimmed
}

func normalizeLocalActorIdentityForStorage(username, baseURL string, actor *activitypub.Actor) *activitypub.Actor {
	if actor == nil {
		return nil
	}

	canonical := canonicalNumericMappingUsername(username)
	if canonical == "" {
		return actor
	}

	normalizedBaseURL := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	normalized := activitypubutil.BuildLocalActor(canonical, normalizedBaseURL, nil, actor)
	if normalized == nil {
		return actor
	}

	normalized.PreferredUsername = canonical
	if normalizedBaseURL == "" {
		normalized.ID = normalizeCanonicalActorReference(normalized.ID, canonical)
		normalized.URL = normalizeCanonicalActorReference(normalized.URL, canonical)
		normalized.Inbox = normalizeCanonicalActorReference(normalized.Inbox, canonical)
		normalized.Outbox = normalizeCanonicalActorReference(normalized.Outbox, canonical)
		normalized.Followers = normalizeCanonicalActorReference(normalized.Followers, canonical)
		normalized.Following = normalizeCanonicalActorReference(normalized.Following, canonical)
		normalized.Liked = normalizeCanonicalActorReference(normalized.Liked, canonical)
		return normalized
	}

	activitypubutil.ApplyLocalActorIdentifiers(normalized, normalizedBaseURL, canonical)

	return normalized
}
