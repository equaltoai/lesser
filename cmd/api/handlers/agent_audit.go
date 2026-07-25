package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

func (h *Handler) recordAgentAuditEvent(ctx *apptheory.Context, claims *auth.Claims, action string, targetID string, metadata map[string]any) {
	if h == nil || h.repos == nil || h.repos.Audit() == nil || claims == nil || !claims.IsAgent {
		return
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
		if targetID != "" {
			metadata["target_id"] = targetID
		}
		if raw, err := json.Marshal(metadata); err == nil {
			entry.Metadata = string(raw)
		} else {
			h.logger.Debug("failed to marshal agent audit metadata", zap.Error(err))
		}
	} else if targetID != "" {
		raw, _ := json.Marshal(map[string]any{"target_id": targetID})
		entry.Metadata = string(raw)
	}

	if err := h.repos.Audit().StoreAuditLog(ctx.Context(), entry); err != nil {
		h.logger.Debug("failed to store agent audit log", zap.Error(err))
	}
}
