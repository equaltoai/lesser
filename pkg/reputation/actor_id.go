package reputation

import (
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
)

// ValidateActorID validates a canonical ActivityPub actor URI used by reputation lookups.
func ValidateActorID(actorID string) error {
	_, err := common.CanonicalActorID(actorID)
	return err
}

func actorIDHost(actorID string) string {
	parsed, err := url.Parse(strings.TrimSpace(actorID))
	if err != nil || parsed == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}
