package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

const (
	agentMaxMemoryCitations = 20
	agentMaxTriggerDetails  = 500
)

var allowedAgentTriggerTypes = map[string]struct{}{
	"scheduled":     {},
	"mention":       {},
	"hashtag_watch": {},
	"manual":        {},
}

func (h *Handler) normalizeAgentMemoryEventRequest(ctx *apptheory.Context, claims *auth.Claims, req *models.CreateStatusRequest) (*apptheory.Response, error) {
	if h == nil || ctx == nil || claims == nil || req == nil || !claims.IsAgent || req.MemoryEvent == nil {
		return nil, nil
	}
	if h.repos == nil || h.repos.Status() == nil {
		resp, respErr := apptheory.JSON(http.StatusServiceUnavailable, map[string]any{
			"error":             "service_unavailable",
			"error_description": "storage is not initialized",
		})
		return resp, respErr
	}

	eventType := strings.ToLower(strings.TrimSpace(req.MemoryEvent.EventType))
	if eventType == "" {
		return nil, nil
	}

	switch eventType {
	case "correction", "retraction":
	default:
		return common.RespondValidationError(ctx, common.ValidationError{
			Field:   "memory_event.event_type",
			Message: "must be one of: correction, retraction",
		})
	}

	originalID := strings.TrimSpace(req.MemoryEvent.OriginalID)
	if originalID == "" {
		originalID = strings.TrimSpace(req.InReplyToID)
	}
	if originalID == "" {
		return common.RespondValidationError(ctx, common.ValidationError{
			Field:   "memory_event.original_id",
			Message: "is required for corrections/retractions",
		})
	}
	if err := common.ValidateStatusID(originalID); err != nil {
		return common.RespondValidationError(ctx, common.ValidationError{
			Field:   "memory_event.original_id",
			Message: "is invalid",
		})
	}

	// Corrections/retractions must reference agent-owned posts.
	original, err := h.repos.Status().GetStatus(ctx.Context(), originalID)
	if err != nil || original == nil {
		return common.RespondNotFound(ctx, "original status not found")
	}
	if !strings.EqualFold(original.AuthorUsername, claims.Username) {
		return apptheory.JSON(http.StatusForbidden, map[string]any{
			"error":             "agent_memory_forbidden",
			"error_description": "agents can only correct or retract their own posts",
		})
	}

	// Ensure the correction/retraction is threaded under the original.
	if strings.TrimSpace(req.InReplyToID) == "" {
		req.InReplyToID = originalID
	}

	req.MemoryEvent.EventType = eventType
	req.MemoryEvent.OriginalID = originalID

	return nil, nil
}

func (h *Handler) buildAgentStatusAttribution(ctx *apptheory.Context, claims *auth.Claims, req *models.CreateStatusRequest) (*activitypub.AgentPostAttribution, *apptheory.Response, error) {
	if h == nil || ctx == nil || claims == nil || req == nil || !claims.IsAgent {
		return nil, nil, nil
	}

	if h.repos == nil || h.repos.User() == nil {
		resp, respErr := apptheory.JSON(http.StatusServiceUnavailable, map[string]any{
			"error":             "service_unavailable",
			"error_description": "storage is not initialized",
		})
		return nil, resp, respErr
	}

	agentUser, _ := h.repos.User().GetUser(ctx.Context(), claims.Username)
	if agentUser == nil || !agentUser.IsAgent {
		resp, respErr := apptheory.JSON(http.StatusForbidden, map[string]any{
			"error":             "agent_required",
			"error_description": "agent attribution is only available for agent accounts",
		})
		return nil, resp, respErr
	}

	triggerType := "manual"
	triggerDetails := ""
	var memoryCitations []string

	if req.AgentAttribution != nil {
		if t := strings.ToLower(strings.TrimSpace(req.AgentAttribution.TriggerType)); t != "" {
			triggerType = t
		}
		triggerDetails = strings.TrimSpace(req.AgentAttribution.TriggerDetails)
		if triggerDetails != "" {
			if err := common.ValidateStringLength("trigger_details", triggerDetails, 0, agentMaxTriggerDetails); err != nil {
				resp, respErr := common.RespondValidationError(ctx, common.ValidationError{
					Field:   "agent_attribution.trigger_details",
					Message: "is too long",
				})
				return nil, resp, respErr
			}
		}

		memoryCitations = normalizeMemoryCitations(req.AgentAttribution.MemoryCitations)
		if len(memoryCitations) > agentMaxMemoryCitations {
			resp, respErr := common.RespondValidationError(ctx, common.ValidationError{
				Field:   "agent_attribution.memory_citations",
				Message: "has too many entries",
			})
			return nil, resp, respErr
		}
		for _, id := range memoryCitations {
			if err := common.ValidateStatusID(id); err != nil {
				resp, respErr := common.RespondValidationError(ctx, common.ValidationError{
					Field:   "agent_attribution.memory_citations",
					Message: "contains an invalid status id",
				})
				return nil, resp, respErr
			}
		}
	}

	if _, ok := allowedAgentTriggerTypes[triggerType]; !ok {
		resp, respErr := common.RespondValidationError(ctx, common.ValidationError{
			Field:   "agent_attribution.trigger_type",
			Message: "must be one of: scheduled, mention, hashtag_watch, manual",
		})
		return nil, resp, respErr
	}

	delegatedBy := strings.TrimSpace(claims.DelegatedBy)
	if delegatedBy == "" {
		delegatedBy = strings.TrimSpace(agentUser.AgentOwner)
	}
	delegatedBy = h.normalizeDelegatedByActorURI(delegatedBy)

	modelID := strings.TrimSpace(agentUser.AgentVersion)
	if modelID == "" {
		modelID = agentVersionUnknown
	}

	attribution := &activitypub.AgentPostAttribution{
		TriggerType:     triggerType,
		TriggerDetails:  triggerDetails,
		MemoryCitations: memoryCitations,
		DelegatedBy:     delegatedBy,
		Scopes:          append([]string(nil), claims.Scopes...),
		Constraints:     buildAgentCapabilityConstraints(agentUser.AgentCapabilities),
		SchemaVersion:   activitypub.AgentAttributionSchemaVersion,
		ModelID:         modelID,
	}

	return attribution, nil, nil
}

func normalizeMemoryCitations(in []string) []string {
	if len(in) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		key := strings.ToLower(raw)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, raw)
	}

	return out
}

func buildAgentCapabilityConstraints(caps *agents.Capabilities) []string {
	if caps == nil {
		return nil
	}

	constraints := make([]string, 0, 4)
	if caps.MaxPostsPerHour > 0 {
		constraints = append(constraints, "max_posts_per_hour:"+strconv.Itoa(caps.MaxPostsPerHour))
	}
	if caps.RequiresApproval {
		constraints = append(constraints, "requires_approval")
	}
	if len(caps.RestrictedDomains) > 0 {
		constraints = append(constraints, "restricted_domains:"+strings.Join(caps.RestrictedDomains, ","))
	}

	return constraints
}
