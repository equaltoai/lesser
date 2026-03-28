package handlers

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

const deprecatedMCPConnectorWarning = `299 - "client_class=agent and agent_username are deprecated for public MCP access; use actor-scoped MCP URLs and standard OAuth registration"`

func usesDeprecatedMCPConnectorSemantics(clientClass, agentUsername string) bool {
	return strings.EqualFold(strings.TrimSpace(clientClass), auth.ClientClassAgent) ||
		strings.TrimSpace(agentUsername) != ""
}

func addDeprecatedMCPConnectorHeaders(resp *apptheory.Response) {
	if resp == nil {
		return
	}

	setHeader(resp, "Deprecation", "true")
	setHeader(resp, "Warning", deprecatedMCPConnectorWarning)
}
