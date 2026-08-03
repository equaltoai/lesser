package federation

import (
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// BuildCanonicalRemoteStatus projects a remote ActivityPub Note into the
// canonical local Status row shape used by product-facing read paths.
func BuildCanonicalRemoteStatus(note *activitypub.Note, localDomain string) *models.Status {
	if note == nil {
		return nil
	}
	local := remoteNoteLocalDomain(localDomain)
	if local == "" {
		// Fail closed: a projection with no valid local domain anchor is never
		// safe, because the local-author rejection below cannot be evaluated.
		return nil
	}
	if remoteNoteAttributionIsLocal(note.AttributedTo, local) {
		return nil
	}
	statusID := models.CanonicalStatusIDForDomain(note.ID, localDomain)
	if statusID == "" {
		return nil
	}

	noteCopy := *note
	status := &models.Status{
		StatusID:       statusID,
		Note:           &noteCopy,
		AuthorID:       strings.TrimSpace(note.AttributedTo),
		AuthorUsername: remoteStatusAuthorUsername(note, localDomain),
		URLs:           remoteStatusProjectionURLs(note),
	}

	if note.Published != nil {
		status.PublishedAt = *note.Published
	}
	if note.Updated != nil {
		status.UpdatedAt = *note.Updated
	}

	return status
}

// remoteNoteAttributionIsLocal decides whether an attributed actor must be
// treated as local, in which case the note must never be projected from a
// remote-fed object.
//
// The primary test parses with net/url — the same parser family the fanout
// consumer (StreamRouterHandler.isLocalActorID) parses with, including its
// scheme lowercasing, Hostname() extraction with Host fallback, and
// case-insensitive host compare. On top of that the guard trims surrounding
// whitespace, fails closed on parse errors and empty input, and adds an
// additive canonical branch, so the guard rejects a strict superset of the
// IDs the consumer treats as local: consumer-local always implies
// guard-local. The remaining divergence is therefore one-directional in the
// safe direction only (e.g. a whitespace-padded or unparseable ID is local
// here but not to the consumer).
//
// The canonical branch is independent defense: CanonicalActorID normalizes
// the scheme, so it covers case-variant schemes even if the primary parse
// ever diverged from the consumer again. A canonicalization failure
// contributes no rejection and falls through to the host compare, so honest
// remote path shapes (/profile/<name>, /actor, /api/v1/accounts/<name>) still
// project — their host is not local, so they cannot reach local stream keys.
func remoteNoteAttributionIsLocal(attributedTo, normalizedLocalDomain string) bool {
	if canonical, err := common.CanonicalActorID(attributedTo); err == nil {
		if domain := normalizeActorDomain(common.ExtractDomainFromActorID(canonical)); domain != "" && domain == normalizedLocalDomain {
			return true
		}
	}
	trimmed := strings.TrimSpace(attributedTo)
	if trimmed == "" {
		return true // no attribution is never safe to project
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return true // fail closed
	}
	host := u.Hostname()
	if host == "" {
		host = u.Host
	}
	return strings.EqualFold(strings.TrimSpace(host), normalizedLocalDomain)
}

// remoteNoteLocalDomain normalizes the configured local domain (with or
// without scheme) and fails closed on any value that cannot be a valid host.
func remoteNoteLocalDomain(localDomain string) string {
	local := normalizeActorDomain(localDomain)
	if local == "" {
		return ""
	}
	u, err := url.Parse("https://" + local)
	if err != nil || u.Hostname() != local {
		return ""
	}
	return local
}

func remoteStatusAuthorUsername(note *activitypub.Note, localDomain string) string {
	if note == nil {
		return ""
	}

	identity := DescribeActorIdentity(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: strings.TrimSpace(note.AttributedTo),
		},
	}, localDomain)

	if identity.IsRemote && identity.Acct != "" {
		return identity.Acct
	}
	if identity.Username != "" {
		return identity.Username
	}

	return usernameFromActorPath(note.AttributedTo)
}

func remoteStatusProjectionURLs(note *activitypub.Note) []string {
	if note == nil {
		return nil
	}

	urls := make([]string, 0, 1)
	if noteID := strings.TrimSpace(note.ID); noteID != "" {
		urls = append(urls, noteID)
	}

	return urls
}
