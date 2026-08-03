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

// remoteNoteAttributionIsLocal compares the attributed actor's host with this
// instance using the same lenient host extraction the fanout consumers apply
// (url.Parse host, case-insensitive, agnostic to path shape, query, fragment,
// userinfo, and port). Guard and consumer must agree by construction: any
// actor ID the stream router treats as local must be rejected here. Remote
// actors with non-canonical path shapes (/profile/<name>, /actor, etc.) still
// project — their host is not local, so they cannot reach local stream keys.
func remoteNoteAttributionIsLocal(attributedTo, normalizedLocalDomain string) bool {
	actorDomain := normalizeActorDomain(common.ExtractDomainFromActorID(strings.TrimSpace(attributedTo)))
	return actorDomain != "" && actorDomain == normalizedLocalDomain
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
