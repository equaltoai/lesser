package reputation

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
)

// ValidateActorID validates a canonical ActivityPub actor URI used by reputation lookups.
func ValidateActorID(actorID string) error {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return fmt.Errorf("actor ID is required")
	}
	if containsControlRune(actorID) || strings.ContainsAny(actorID, " \t\r\n") {
		return fmt.Errorf("actor ID contains invalid characters")
	}
	if err := common.ValidateActivityPubURL(actorID, "actor_id"); err != nil {
		return err
	}

	parsed, err := url.Parse(actorID)
	if err != nil || parsed == nil {
		return fmt.Errorf("actor ID must be a valid URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("actor ID must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("actor ID must not contain query or fragment")
	}
	if parsed.Opaque != "" || strings.TrimSpace(parsed.Hostname()) == "" || strings.TrimSpace(parsed.Path) == "" || parsed.Path == "/" {
		return fmt.Errorf("actor ID must include host and path")
	}
	for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
		if segment == "" {
			continue
		}
		unescaped, err := url.PathUnescape(segment)
		if err != nil {
			return fmt.Errorf("actor ID path contains invalid escaping")
		}
		if unescaped == "." || unescaped == ".." || containsControlRune(unescaped) {
			return fmt.Errorf("actor ID path contains unsafe segment")
		}
	}
	return nil
}

func containsControlRune(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
