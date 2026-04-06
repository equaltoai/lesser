package federation

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// BuildCanonicalRemoteStatus projects a remote ActivityPub Note into the
// canonical local Status row shape used by product-facing read paths.
func BuildCanonicalRemoteStatus(note *activitypub.Note, localDomain string) *models.Status {
	if note == nil {
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
