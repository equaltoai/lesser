package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

const (
	agentMemorySearchDefaultLimit = 20
	agentMemorySearchMaxLimit     = 50
	agentMemorySearchEventCap     = 2000
)

// HandleAgentMemorySearchLift handles GET/POST /api/v1/agents/memory/search.
//
// M4: timeline-as-memory retrieval for agent-scoped queries.
func (h *Handler) HandleAgentMemorySearchLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	if claims == nil || !claims.IsAgent {
		return common.RespondForbidden(ctx, "agent token required")
	}

	req := models.AgentMemorySearchRequest{}
	if strings.EqualFold(ctx.Request.Method, http.MethodGet) {
		req.Query = queryValue(ctx, "query")
		req.ThreadID = queryValue(ctx, "thread_id")
		req.IncludeThreads = strings.EqualFold(queryValue(ctx, "include_threads"), "true")
		if v, parseErr := common.ParseAndValidateIntWithBounds("limit", queryValue(ctx, "limit"), 0, agentMemorySearchMaxLimit, agentMemorySearchDefaultLimit); parseErr == nil {
			req.Limit = v
		} else {
			req.Limit = agentMemorySearchDefaultLimit
		}

		if tagsRaw := strings.TrimSpace(queryValue(ctx, "tags")); tagsRaw != "" {
			req.Tags = splitCommaList(tagsRaw)
		}

		since := strings.TrimSpace(queryValue(ctx, "since_date"))
		until := strings.TrimSpace(queryValue(ctx, "until_date"))
		if since != "" || until != "" {
			req.DateRange = &models.DateRange{Start: since, End: until}
		}
	} else {
		if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
			return common.RespondBadRequest(ctx, "invalid request body")
		}
	}

	limit := req.Limit
	if limit <= 0 {
		limit = agentMemorySearchDefaultLimit
	}
	if limit > agentMemorySearchMaxLimit {
		limit = agentMemorySearchMaxLimit
	}

	start := time.Now()
	results, err := h.searchAgentMemory(ctx, claims.Username, req, limit)
	if err != nil {
		var vErr common.ValidationError
		if errors.As(err, &vErr) {
			return common.RespondValidationError(ctx, err)
		}
		h.logger.Error("agent memory search failed", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	respPayload := models.AgentMemorySearchResponse{
		Results:     results,
		Total:       len(results),
		QueryTimeMS: int(time.Since(start).Milliseconds()),
	}

	return okJSON(respPayload)
}

func (h *Handler) searchAgentMemory(ctx *apptheory.Context, agentUsername string, req models.AgentMemorySearchRequest, limit int) ([]models.AgentMemorySearchResult, error) {
	if h == nil || ctx == nil || h.repos == nil || h.repos.GetDB() == nil {
		return nil, fmt.Errorf("storage unavailable")
	}

	agentUsername = strings.TrimSpace(agentUsername)
	if agentUsername == "" {
		return nil, fmt.Errorf("agent username required")
	}

	normalizedTags := normalizeTags(req.Tags)
	since, until, err := parseDateRange(req.DateRange)
	if err != nil {
		return nil, err
	}

	// Thread lookup mode: return the agent's view of a single conversation thread.
	if strings.TrimSpace(req.ThreadID) != "" {
		return h.searchAgentMemoryThread(ctx, agentUsername, strings.TrimSpace(req.ThreadID), limit)
	}

	db := h.repos.GetDB()

	queryCap := agentMemorySearchEventCap
	if limit > 0 && limit*50 > queryCap {
		queryCap = limit * 50
	}
	if queryCap > agentMemorySearchEventCap {
		queryCap = agentMemorySearchEventCap
	}

	var events []storageModels.AgentMemoryEvent
	err = db.WithContext(ctx.Context()).Model(&storageModels.AgentMemoryEvent{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("AGENT#%s", agentUsername)).
		OrderBy("gsi1SK", "DESC").
		Limit(queryCap).
		All(&events)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return []models.AgentMemorySearchResult{}, nil
		}
		return nil, err
	}

	seen := make(map[string]struct{}, len(events))
	out := make([]models.AgentMemorySearchResult, 0, limit)

	for _, event := range events {
		originalID := strings.TrimSpace(event.OriginalID)
		if originalID == "" {
			continue
		}
		if _, ok := seen[originalID]; ok {
			continue
		}
		seen[originalID] = struct{}{}

		if strings.EqualFold(event.EventType, storageModels.MemoryEventTombstone) {
			continue
		}

		statusID := strings.TrimSpace(event.StatusID)
		if statusID == "" {
			continue
		}

		status, err := h.repos.Status().GetStatus(ctx.Context(), statusID)
		if err != nil || status == nil || status.Deleted {
			continue
		}

		if since != nil && status.PublishedAt.Before(*since) {
			continue
		}
		if until != nil && status.PublishedAt.After(*until) {
			continue
		}

		if len(normalizedTags) > 0 && !statusHasAllTags(status.Hashtags, normalizedTags) {
			continue
		}

		score := relevanceScore(req.Query, status.Content)
		if strings.TrimSpace(req.Query) != "" && score <= 0 {
			continue
		}

		apiStatus, err := h.convertStorageStatusToAPI(status, agentUsername)
		if err != nil {
			continue
		}

		result := models.AgentMemorySearchResult{
			Status:         apiStatus,
			RelevanceScore: score,
			Context: &models.AgentMemorySearchContext{
				ThreadRoot: status.ConversationID,
				ReplyCount: status.ReplyCount,
				Tags:       append([]string(nil), status.Hashtags...),
				EventType:  event.EventType,
				OriginalID: originalID,
			},
		}

		if req.IncludeThreads {
			result.Thread, _ = h.fetchAgentThread(ctx, agentUsername, status)
		}

		out = append(out, result)
		if len(out) >= limit {
			break
		}
	}

	return out, nil
}

func (h *Handler) searchAgentMemoryThread(ctx *apptheory.Context, agentUsername string, threadID string, limit int) ([]models.AgentMemorySearchResult, error) {
	if h == nil || ctx == nil || h.repos == nil || h.repos.Status() == nil {
		return nil, fmt.Errorf("storage unavailable")
	}

	rootID := threadID
	if rootID == "" {
		return []models.AgentMemorySearchResult{}, nil
	}

	// Fetch the thread via CONVERSATION#... and filter to agent-authored items.
	thread, err := h.repos.Status().GetConversationThread(ctx.Context(), rootID, interfaces.PaginationOptions{Limit: 100})
	if err != nil || thread == nil {
		return []models.AgentMemorySearchResult{}, nil
	}

	filtered := make([]*storageModels.Status, 0, len(thread.Items))
	for _, status := range thread.Items {
		if status == nil || status.Deleted {
			continue
		}
		if !strings.EqualFold(status.AuthorUsername, agentUsername) {
			continue
		}
		filtered = append(filtered, status)
	}

	if len(filtered) == 0 {
		return []models.AgentMemorySearchResult{}, nil
	}

	capped := capThreadForAgent(filtered, 21)
	apiThread := make([]*models.Status, 0, len(capped))
	for _, s := range capped {
		apiStatus, err := h.convertStorageStatusToAPI(s, agentUsername)
		if err != nil {
			continue
		}
		apiThread = append(apiThread, apiStatus)
	}

	if len(apiThread) == 0 {
		return []models.AgentMemorySearchResult{}, nil
	}

	root := apiThread[0]
	return []models.AgentMemorySearchResult{
		{
			Status:         root,
			RelevanceScore: 1,
			Context: &models.AgentMemorySearchContext{
				ThreadRoot: rootID,
			},
			Thread: apiThread,
		},
	}, nil
}

func (h *Handler) fetchAgentThread(ctx *apptheory.Context, agentUsername string, status *storageModels.Status) ([]*models.Status, error) {
	if h == nil || ctx == nil || h.repos == nil || h.repos.Status() == nil || status == nil {
		return nil, nil
	}

	threadID := strings.TrimSpace(status.ConversationID)
	if threadID == "" {
		threadID = status.StatusID
	}

	thread, err := h.repos.Status().GetConversationThread(ctx.Context(), threadID, interfaces.PaginationOptions{Limit: 100})
	if err != nil || thread == nil {
		return nil, nil
	}

	filtered := make([]*storageModels.Status, 0, len(thread.Items))
	for _, s := range thread.Items {
		if s == nil || s.Deleted {
			continue
		}
		if !strings.EqualFold(s.AuthorUsername, agentUsername) {
			continue
		}
		filtered = append(filtered, s)
	}

	capped := capThreadForAgent(filtered, 21)
	apiThread := make([]*models.Status, 0, len(capped))
	for _, s := range capped {
		apiStatus, err := h.convertStorageStatusToAPI(s, agentUsername)
		if err != nil {
			continue
		}
		apiThread = append(apiThread, apiStatus)
	}

	return apiThread, nil
}

func capThreadForAgent(statuses []*storageModels.Status, limit int) []*storageModels.Status {
	if len(statuses) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 21
	}
	if len(statuses) <= limit {
		return statuses
	}

	root := statuses[0]
	tail := statuses[len(statuses)-(limit-1):]
	if len(tail) > 0 && tail[0] != nil && root != nil && tail[0].StatusID == root.StatusID {
		return tail
	}
	out := make([]*storageModels.Status, 0, 1+len(tail))
	out = append(out, root)
	out = append(out, tail...)
	return out
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = mastodon.NormalizeHashtag(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func statusHasAllTags(statusTags []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	if len(statusTags) == 0 {
		return false
	}

	set := make(map[string]struct{}, len(statusTags))
	for _, tag := range statusTags {
		set[mastodon.NormalizeHashtag(tag)] = struct{}{}
	}

	for _, tag := range required {
		if _, ok := set[tag]; !ok {
			return false
		}
	}
	return true
}

func relevanceScore(query string, content string) float64 {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 1
	}

	content = strings.ToLower(content)
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 1
	}

	matched := 0
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(content, term) {
			matched++
		}
	}

	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(terms))
}

func parseDateRange(r *models.DateRange) (*time.Time, *time.Time, error) {
	if r == nil {
		return nil, nil, nil
	}

	var (
		start *time.Time
		end   *time.Time
	)

	if s := strings.TrimSpace(r.Start); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return nil, nil, common.ValidationError{Field: "date_range.start", Message: "must be YYYY-MM-DD"}
		}
		utc := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		start = &utc
	}

	if s := strings.TrimSpace(r.End); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return nil, nil, common.ValidationError{Field: "date_range.end", Message: "must be YYYY-MM-DD"}
		}
		utc := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC)
		end = &utc
	}

	return start, end, nil
}

func splitCommaList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
