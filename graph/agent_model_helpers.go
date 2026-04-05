package graph

import (
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
)

const agentVersionUnknown = "unknown"

func (r *Resolver) convertStorageUserToAgent(user *storage.User, governance *storage.AgentGovernanceState) *model.Agent {
	if user == nil {
		return nil
	}

	username := strings.TrimSpace(user.Username)
	if username == "" {
		return nil
	}

	id := username
	baseURL := ""
	if r != nil && r.Config != nil {
		id = r.Config.ActorURL(username)
		baseURL = r.Config.BaseURL()
	}

	displayName := strings.TrimSpace(user.DisplayName)
	if displayName == "" {
		displayName = username
	}

	var bio *string
	if v := strings.TrimSpace(user.Note); v != "" {
		bio = &v
	}

	agentType := normalizeAgentType(user.AgentType)
	agentVersion := strings.TrimSpace(user.AgentVersion)
	if agentVersion == "" {
		agentVersion = agentVersionUnknown
	}

	capabilities := activitypubAgentCapabilitiesFromStorage(user.AgentCapabilities)
	delegatedScopes := graphAgentDelegatedScopes(governance)
	if delegatedScopes == nil {
		delegatedScopes = []string{}
	}

	verified := graphAgentVerifiedState(governance)
	verifiedAt := graphAgentVerifiedAt(governance)

	var agentOwner *string
	if v := strings.TrimSpace(user.AgentOwner); v != "" {
		agentOwner = &v
	}

	createdAt := time.Now().UTC()
	if !user.CreatedAt.IsZero() {
		createdAt = user.CreatedAt
	}

	quarantineStatus, quarantineStart, quarantineEnd, quarantineApprovedBy, quarantineApprovedAt, quarantineActive := graphAgentQuarantineFields(governance)

	return &model.Agent{
		ID:                   id,
		Username:             username,
		DisplayName:          displayName,
		Bio:                  bio,
		AgentType:            agentType,
		AgentVersion:         agentVersion,
		AgentCapabilities:    capabilities,
		AgentOwner:           agentOwner,
		DelegatedScopes:      delegatedScopes,
		McpAccess:            graphAgentMCPAccessModel(auth.BuildPublicMCPAccessBundle(baseURL, username)),
		Verified:             verified,
		VerifiedAt:           verifiedAt,
		QuarantineStatus:     quarantineStatus,
		QuarantineStart:      quarantineStart,
		QuarantineEnd:        quarantineEnd,
		QuarantineApprovedBy: quarantineApprovedBy,
		QuarantineApprovedAt: quarantineApprovedAt,
		QuarantineActive:     quarantineActive,
		OwnerActor:           nil,
		Type:                 agentType,
		Version:              agentVersion,
		Capabilities:         capabilities,
		Owner:                nil,
		CreatedAt:            model.Time(createdAt),
		ActivityCount:        0,
	}
}

func graphAgentQuarantineFields(governance *storage.AgentGovernanceState) (*string, *model.Time, *model.Time, *string, *model.Time, bool) {
	summary := governance.QuarantineSummaryAt(time.Now().UTC())
	return graphOptionalString(summary.Status),
		graphOptionalModelTime(summary.Start),
		graphOptionalModelTime(summary.End),
		graphOptionalString(summary.ApprovedBy),
		graphOptionalModelTime(summary.ApprovedAt),
		summary.Active
}

func graphOptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func graphOptionalModelTime(value *time.Time) *model.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	timestamp := model.Time(value.UTC())
	return &timestamp
}

func graphAgentMCPAccessModel(bundle auth.PublicMCPAccessBundle) *model.AgentMCPAccess {
	return &model.AgentMCPAccess{
		McpURL:                 bundle.MCPURL,
		ProtectedResourceURL:   bundle.ProtectedResourceURL,
		AuthorizationServerURL: bundle.AuthorizationServerURL,
		RegistrationURL:        bundle.RegistrationURL,
		Scopes:                 append([]string(nil), bundle.SupportedScopes...),
		Guidance:               append([]string(nil), bundle.Guidance...),
	}
}

func normalizeAgentType(value string) model.AgentType {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(model.AgentTypeCurator):
		return model.AgentTypeCurator
	case string(model.AgentTypeModerator):
		return model.AgentTypeModerator
	case string(model.AgentTypeResearcher):
		return model.AgentTypeResearcher
	case string(model.AgentTypeAssistant):
		return model.AgentTypeAssistant
	case string(model.AgentTypeBridge):
		return model.AgentTypeBridge
	default:
		return model.AgentTypeCustom
	}
}

func activitypubAgentCapabilitiesFromStorage(caps *agents.Capabilities) *activitypub.AgentCapabilities {
	if caps == nil {
		return &activitypub.AgentCapabilities{}
	}

	return &activitypub.AgentCapabilities{
		CanPost:           caps.CanPost,
		CanReply:          caps.CanReply,
		CanBoost:          caps.CanBoost,
		CanFollow:         caps.CanFollow,
		CanDM:             caps.CanDM,
		RestrictedDomains: append([]string(nil), caps.RestrictedDomains...),
		MaxPostsPerHour:   caps.MaxPostsPerHour,
		RequiresApproval:  caps.RequiresApproval,
	}
}

func graphAgentVerifiedState(governance *storage.AgentGovernanceState) bool {
	return governance != nil && governance.Verified
}

func graphAgentVerifiedAt(governance *storage.AgentGovernanceState) *model.Time {
	if governance == nil || !governance.Verified || governance.VerifiedAt == nil || governance.VerifiedAt.IsZero() {
		return nil
	}
	timestamp := model.Time(governance.VerifiedAt.UTC())
	return &timestamp
}

func graphAgentDelegatedScopes(governance *storage.AgentGovernanceState) []string {
	if governance == nil {
		return nil
	}
	return governance.DelegatedScopesCopy()
}
