package graph

import (
	"context"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/storage"
)

const agentVersionUnknown = "unknown"

func (r *Resolver) convertStorageUserToAgent(_ context.Context, user *storage.User) *model.Agent {
	if user == nil {
		return nil
	}

	username := strings.TrimSpace(user.Username)
	if username == "" {
		return nil
	}

	id := username
	if r != nil && r.Config != nil {
		id = r.Config.ActorURL(username)
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
	delegatedScopes := agentDelegatedScopes(user)
	if delegatedScopes == nil {
		delegatedScopes = []string{}
	}

	verified := agentMetadataBool(user, "agent_verified")
	var verifiedAt *model.Time
	if raw, ok := agentMetadataString(user, "agent_verified_at"); ok {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw)); err == nil {
			t := model.Time(parsed)
			verifiedAt = &t
		}
	}

	var agentOwner *string
	if v := strings.TrimSpace(user.AgentOwner); v != "" {
		agentOwner = &v
	}

	createdAt := time.Now().UTC()
	if !user.CreatedAt.IsZero() {
		createdAt = user.CreatedAt
	}

	return &model.Agent{
		ID:                id,
		Username:          username,
		DisplayName:       displayName,
		Bio:               bio,
		AgentType:         agentType,
		AgentVersion:      agentVersion,
		AgentCapabilities: capabilities,
		AgentOwner:        agentOwner,
		DelegatedScopes:   delegatedScopes,
		Verified:          verified,
		VerifiedAt:        verifiedAt,
		OwnerActor:        nil,
		Type:              agentType,
		Version:           agentVersion,
		Capabilities:      capabilities,
		Owner:             nil,
		CreatedAt:         model.Time(createdAt),
		ActivityCount:     0,
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

func agentMetadataBool(user *storage.User, key string) bool {
	if user == nil || user.Metadata == nil {
		return false
	}
	raw, ok := user.Metadata[key]
	if !ok || raw == nil {
		return false
	}

	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func agentMetadataString(user *storage.User, key string) (string, bool) {
	if user == nil || user.Metadata == nil {
		return "", false
	}
	raw, ok := user.Metadata[key]
	if !ok || raw == nil {
		return "", false
	}
	if v, ok := raw.(string); ok {
		v = strings.TrimSpace(v)
		if v == "" {
			return "", false
		}
		return v, true
	}
	return "", false
}

func agentDelegatedScopes(user *storage.User) []string {
	if user == nil || user.Metadata == nil {
		return nil
	}

	raw, ok := user.Metadata["agent_delegated_scopes"]
	if !ok || raw == nil {
		return nil
	}

	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			if s := strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
