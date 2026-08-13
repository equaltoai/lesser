package handlers

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

func (h *Handler) recordAgentAuditEvent(ctx *apptheory.Context, claims *auth.Claims, action string, targetID string, metadata map[string]any) {
	if h == nil || h.repos == nil || h.repos.Audit() == nil || claims == nil {
		return
	}

	// Grantee-driven agent actions arrive under a grantee-subject token whose
	// audience is the shared agent's actor MCP URL. They must not silently drop
	// the audit trail, so admit them and record both identities: the real caller
	// (grantee) in Username and the agent plus session in Metadata.
	agentUsername := ""
	if !claims.IsAgent {
		agent, ok := actorUsernameFromAudience(claims)
		if !ok {
			return
		}
		agentUsername = agent
	}

	action = strings.TrimSpace(action)
	if action == "" {
		return
	}

	now := time.Now().UTC()

	entry := &storageModels.AuthAuditLog{
		ID:        common.GenerateOperationIDULID(),
		Timestamp: now,
		EventType: action,
		Severity:  "INFO",
		Username:  claims.Username,
		SessionID: claims.AgentSessionID,
		IPAddress: headerValue(ctx, "x-forwarded-for"),
		UserAgent: headerValue(ctx, "user-agent"),
		Success:   true,
		CreatedAt: now,
	}

	if entry.SessionID == "" {
		entry.SessionID = claims.SessionID
	}

	if metadata != nil {
		if agentUsername != "" {
			metadata["agent_username"] = agentUsername
			metadata["agent_session_id"] = claims.SessionID
		}
		if targetID != "" {
			metadata["target_id"] = targetID
		}
		if raw, err := json.Marshal(metadata); err == nil {
			entry.Metadata = string(raw)
		} else {
			h.logger.Debug("failed to marshal agent audit metadata", zap.Error(err))
		}
	} else {
		extra := map[string]any{}
		if agentUsername != "" {
			extra["agent_username"] = agentUsername
			extra["agent_session_id"] = claims.SessionID
		}
		if targetID != "" {
			extra["target_id"] = targetID
		}
		if len(extra) > 0 {
			raw, _ := json.Marshal(extra)
			entry.Metadata = string(raw)
		}
	}

	if err := h.repos.Audit().StoreAuditLog(ctx.Context(), entry); err != nil {
		h.logger.Debug("failed to store agent audit log", zap.Error(err))
	}
}

// actorUsernameFromAudience extracts the agent username from a grantee-subject
// token whose single audience is the shared agent's actor MCP URL. It returns
// false for agent-subject tokens and for tokens whose audience is not an actor
// MCP resource.
func actorUsernameFromAudience(claims *auth.Claims) (string, bool) {
	if claims == nil || len(claims.Audience) != 1 {
		return "", false
	}
	parsed, err := url.Parse(strings.TrimSpace(claims.Audience[0]))
	if err != nil || parsed == nil {
		return "", false
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) != 2 || segments[0] != oauthMCPSegment || strings.TrimSpace(segments[1]) == "" {
		return "", false
	}
	return strings.TrimSpace(segments[1]), true
}
