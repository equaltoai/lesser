package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

func (h *Handler) recordAgentMemoryEvent(ctx *apptheory.Context, agentUsername string, statusID string, req *models.CreateStatusRequest) {
	if h == nil || h.repos == nil {
		return
	}

	db := h.repos.GetDB()
	if db == nil {
		return
	}

	agentUsername = strings.TrimSpace(agentUsername)
	statusID = strings.TrimSpace(statusID)
	if agentUsername == "" || statusID == "" {
		return
	}

	eventType := storageModels.MemoryEventCreate
	originalID := statusID
	reason := ""

	if req != nil && req.MemoryEvent != nil {
		switch strings.ToLower(strings.TrimSpace(req.MemoryEvent.EventType)) {
		case storageModels.MemoryEventCorrection:
			eventType = storageModels.MemoryEventCorrection
		case storageModels.MemoryEventRetraction:
			eventType = storageModels.MemoryEventRetraction
		default:
			// ignore unsupported/empty values; treat as create
		}

		if v := strings.TrimSpace(req.MemoryEvent.OriginalID); v != "" {
			originalID = v
		}
		reason = strings.TrimSpace(req.MemoryEvent.Reason)
	}

	entry := &storageModels.AgentMemoryEvent{
		EventID:       common.GenerateOperationIDULID(),
		EventType:     eventType,
		StatusID:      statusID,
		OriginalID:    originalID,
		AgentUsername: agentUsername,
		Reason:        reason,
		Timestamp:     time.Now().UTC(),
	}

	if err := db.Model(entry).WithContext(ctx.Context()).Create(); err != nil {
		h.logger.Debug("failed to store agent memory event", zap.Error(err))
	}
}

func (h *Handler) recordAgentMemoryTombstone(ctx context.Context, agentUsername string, statusID string, reason string) {
	if h == nil || h.repos == nil {
		return
	}

	db := h.repos.GetDB()
	if db == nil {
		return
	}

	agentUsername = strings.TrimSpace(agentUsername)
	statusID = strings.TrimSpace(statusID)
	if agentUsername == "" || statusID == "" {
		return
	}

	entry := &storageModels.AgentMemoryEvent{
		EventID:       common.GenerateOperationIDULID(),
		EventType:     storageModels.MemoryEventTombstone,
		StatusID:      statusID,
		OriginalID:    statusID,
		AgentUsername: agentUsername,
		Reason:        strings.TrimSpace(reason),
		Timestamp:     time.Now().UTC(),
	}

	if err := db.Model(entry).WithContext(ctx).Create(); err != nil {
		h.logger.Debug("failed to store agent tombstone memory event", zap.Error(err))
	}
}
