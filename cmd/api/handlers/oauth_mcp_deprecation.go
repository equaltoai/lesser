package handlers

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
)

func usesDeprecatedMCPConnectorSemantics(clientClass, agentUsername string) bool {
	return strings.EqualFold(strings.TrimSpace(clientClass), auth.ClientClassAgent) ||
		strings.TrimSpace(agentUsername) != ""
}
