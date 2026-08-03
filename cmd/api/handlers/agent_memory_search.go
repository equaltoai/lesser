package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

const (
	agentMemorySearchDefaultLimit = 20
	agentMemorySearchMaxLimit     = 50
	agentMemorySearchEventCap     = 2000
)

type agentMemoryCandidate struct {
	status *storageModels.Status
	score  float64
}

func (h *Handler) authenticateAgentMemorySearch(ctx *apptheory.Context) (*auth.Claims, *apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			resp, respErr := common.RespondForbidden(ctx, err.Error())
			return nil, resp, respErr
		}
		resp, respErr := common.RespondUnauthorized(ctx)
		return nil, resp, respErr
	}
	if claims == nil || !claims.IsAgent {
		resp, respErr := common.RespondForbidden(ctx, "agent token required")
		return nil, resp, respErr
	}
	return claims, nil, nil
}

func parseAgentMemorySearchRequest(ctx *apptheory.Context) (models.AgentMemorySearchRequest, *apptheory.Response, error) {
	req := models.AgentMemorySearchRequest{}
	if strings.EqualFold(ctx.Request.Method, http.MethodGet) {
		req.Query = ctx.Query("query")
		req.Mode = ctx.Query("mode")
		req.ThreadID = ctx.Query("thread_id")
		req.IncludeThreads = strings.EqualFold(ctx.Query("include_threads"), "true")
		if v, parseErr := common.ParseAndValidateIntWithBounds("limit", ctx.Query("limit"), 0, agentMemorySearchMaxLimit, agentMemorySearchDefaultLimit); parseErr == nil {
			req.Limit = v
		} else {
			req.Limit = agentMemorySearchDefaultLimit
		}

		if tagsRaw := strings.TrimSpace(ctx.Query("tags")); tagsRaw != "" {
			req.Tags = splitCommaList(tagsRaw)
		}

		since := strings.TrimSpace(ctx.Query("since_date"))
		until := strings.TrimSpace(ctx.Query("until_date"))
		if since != "" || until != "" {
			req.DateRange = &models.DateRange{Start: since, End: until}
		}
		return req, nil, nil
	}

	bound, err := apptheory.BindRequest[models.AgentMemorySearchRequest](ctx, apptheory.BindConfig[models.AgentMemorySearchRequest]{
		Body: true,
	})
	if err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return models.AgentMemorySearchRequest{}, resp, respErr
	}

	return bound, nil, nil
}

func normalizeAgentMemorySearchLimit(req models.AgentMemorySearchRequest) int {
	limit := req.Limit
	if limit <= 0 {
		limit = agentMemorySearchDefaultLimit
	}
	if limit > agentMemorySearchMaxLimit {
		limit = agentMemorySearchMaxLimit
	}
	return limit
}

func normalizeAgentMemorySearchMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "timeline"
	}
	return mode
}

func (h *Handler) validateAgentMemorySearchMode(ctx *apptheory.Context, mode string) (*apptheory.Response, error) {
	switch mode {
	case "timeline":
		return nil, nil
	case "hybrid":
		if h.repos == nil || h.repos.Instance() == nil {
			return common.RespondForbidden(ctx, "hybrid retrieval is disabled")
		}
		policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context())
		if err != nil || policy == nil || !policy.HybridRetrievalEnabled {
			return common.RespondForbidden(ctx, "hybrid retrieval is disabled by instance policy")
		}
		return nil, nil
	default:
		return common.RespondValidationError(ctx, common.ValidationError{Field: "mode", Message: "must be \"timeline\" or \"hybrid\""})
	}
}

// HandleAgentMemorySearchLift handles GET/POST /api/v1/agents/memory/search.
//
// M4: timeline-as-memory retrieval for agent-scoped queries.
func (h *Handler) HandleAgentMemorySearchLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	claims, resp, err := h.authenticateAgentMemorySearch(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	req, resp, err := parseAgentMemorySearchRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	limit := normalizeAgentMemorySearchLimit(req)
	mode := normalizeAgentMemorySearchMode(req.Mode)
	req.Mode = mode

	if resp, err := h.validateAgentMemorySearchMode(ctx, mode); resp != nil || err != nil {
		return resp, err
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
	if h == nil || ctx == nil || h.repos == nil || h.repos.GetDB() == nil || h.repos.Status() == nil {
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

	events, err := h.listAgentMemoryEvents(ctx.Context(), agentUsername, agentMemorySearchEventCap)
	if err != nil {
		return nil, err
	}

	seenOriginal := make(map[string]struct{}, len(events))
	seenStatusIDs := make(map[string]struct{}, limit)
	out := make([]models.AgentMemorySearchResult, 0, limit)

	for _, event := range events {
		originalID := strings.TrimSpace(event.OriginalID)
		if originalID == "" {
			continue
		}
		if _, ok := seenOriginal[originalID]; ok {
			continue
		}
		seenOriginal[originalID] = struct{}{}

		result, statusID := h.buildAgentMemorySearchResult(ctx, agentUsername, req, event, originalID, since, until, normalizedTags)
		if result == nil {
			continue
		}

		if statusID != "" {
			seenStatusIDs[statusID] = struct{}{}
		}

		out = append(out, *result)
		if len(out) >= limit {
			break
		}
	}

	if strings.EqualFold(strings.TrimSpace(req.Mode), "hybrid") && len(out) < limit && strings.TrimSpace(req.Query) != "" {
		extra, err := h.searchAgentMemoryHybridFallback(ctx, agentUsername, req.Query, since, until, normalizedTags, limit-len(out), seenStatusIDs, req.IncludeThreads)
		if err == nil && len(extra) > 0 {
			out = append(out, extra...)
			if len(out) > limit {
				out = out[:limit]
			}
		}
	}

	return out, nil
}

func (h *Handler) searchAgentMemoryHybridFallback(
	ctx *apptheory.Context,
	agentUsername string,
	query string,
	since *time.Time,
	until *time.Time,
	requiredTags []string,
	limit int,
	seen map[string]struct{},
	includeThreads bool,
) ([]models.AgentMemorySearchResult, error) {
	if h == nil || ctx == nil || h.repos == nil || h.repos.Status() == nil || h.cfg == nil {
		return nil, fmt.Errorf("storage unavailable")
	}
	if limit <= 0 {
		return nil, nil
	}

	maxCandidates := h.hybridRetrievalMaxCandidates(ctx)

	actorID := h.cfg.ActorURL(agentUsername)
	timeline, err := h.repos.Status().GetUserTimeline(ctx.Context(), actorID, interfaces.PaginationOptions{Limit: maxCandidates})
	if err != nil || timeline == nil || len(timeline.Items) == 0 {
		return nil, nil
	}

	candidates := make([]agentMemoryCandidate, 0, len(timeline.Items))
	for _, status := range timeline.Items {
		score, ok := hybridCandidateScore(status, query, since, until, requiredTags, seen)
		if !ok {
			continue
		}
		candidates = append(candidates, agentMemoryCandidate{status: status, score: score})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].status.PublishedAt.After(candidates[j].status.PublishedAt)
		}
		return candidates[i].score > candidates[j].score
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return h.convertHybridCandidatesToResults(ctx, agentUsername, candidates, includeThreads), nil
}

func (h *Handler) listAgentMemoryEvents(ctx context.Context, agentUsername string, limit int) ([]storageModels.AgentMemoryEvent, error) {
	if h == nil || h.repos == nil || h.repos.GetDB() == nil {
		return nil, fmt.Errorf("storage unavailable")
	}

	db := h.repos.GetDB()
	var events []storageModels.AgentMemoryEvent
	err := db.WithContext(ctx).Model(&storageModels.AgentMemoryEvent{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("AGENT#%s", agentUsername)).
		OrderBy("gsi1SK", "DESC").
		Limit(limit).
		All(&events)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return []storageModels.AgentMemoryEvent{}, nil
		}
		return nil, err
	}

	return events, nil
}

func (h *Handler) buildAgentMemorySearchResult(
	ctx *apptheory.Context,
	agentUsername string,
	req models.AgentMemorySearchRequest,
	event storageModels.AgentMemoryEvent,
	originalID string,
	since *time.Time,
	until *time.Time,
	requiredTags []string,
) (*models.AgentMemorySearchResult, string) {
	if strings.EqualFold(event.EventType, storageModels.MemoryEventTombstone) {
		return nil, ""
	}

	statusID := strings.TrimSpace(event.StatusID)
	if statusID == "" {
		return nil, ""
	}

	status, err := h.repos.Status().GetStatus(ctx.Context(), statusID)
	if err != nil || status == nil || status.Deleted {
		return nil, ""
	}

	if since != nil && status.PublishedAt.Before(*since) {
		return nil, ""
	}
	if until != nil && status.PublishedAt.After(*until) {
		return nil, ""
	}

	if len(requiredTags) > 0 && !statusHasAllTags(status.Hashtags, requiredTags) {
		return nil, ""
	}

	score := relevanceScore(req.Query, status.Content)
	if strings.TrimSpace(req.Query) != "" && score <= 0 {
		return nil, ""
	}

	apiStatus, err := h.convertStorageStatusToAPI(status, agentUsername)
	if err != nil {
		return nil, ""
	}

	result := &models.AgentMemorySearchResult{
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

	return result, statusID
}

func (h *Handler) hybridRetrievalMaxCandidates(ctx *apptheory.Context) int {
	const defaultMax = 200

	maxCandidates := defaultMax
	if h != nil && h.repos != nil && h.repos.Instance() != nil && ctx != nil {
		if policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context()); err == nil && policy != nil && policy.HybridRetrievalMaxCandidates > 0 {
			maxCandidates = policy.HybridRetrievalMaxCandidates
		}
	}
	if maxCandidates <= 0 {
		maxCandidates = defaultMax
	}
	if maxCandidates > agentMemorySearchEventCap {
		maxCandidates = agentMemorySearchEventCap
	}
	return maxCandidates
}

func hybridCandidateScore(status *storageModels.Status, query string, since, until *time.Time, requiredTags []string, seen map[string]struct{}) (float64, bool) {
	if status == nil || status.Deleted {
		return 0, false
	}
	if seen != nil {
		if _, ok := seen[status.StatusID]; ok {
			return 0, false
		}
	}
	if since != nil && status.PublishedAt.Before(*since) {
		return 0, false
	}
	if until != nil && status.PublishedAt.After(*until) {
		return 0, false
	}
	if len(requiredTags) > 0 && !statusHasAllTags(status.Hashtags, requiredTags) {
		return 0, false
	}

	score := relevanceScore(query, status.Content)
	if score <= 0 {
		return 0, false
	}

	return score, true
}

func (h *Handler) convertHybridCandidatesToResults(ctx *apptheory.Context, agentUsername string, candidates []agentMemoryCandidate, includeThreads bool) []models.AgentMemorySearchResult {
	out := make([]models.AgentMemorySearchResult, 0, len(candidates))
	for _, entry := range candidates {
		apiStatus, err := h.convertStorageStatusToAPI(entry.status, agentUsername)
		if err != nil {
			continue
		}

		result := models.AgentMemorySearchResult{
			Status:         apiStatus,
			RelevanceScore: entry.score,
		}

		if includeThreads {
			result.Thread, _ = h.fetchAgentThread(ctx, agentUsername, entry.status)
		}

		out = append(out, result)
	}

	return out
}

func (h *Handler) searchAgentMemoryThread(ctx *apptheory.Context, agentUsername string, threadID string, _ int) ([]models.AgentMemorySearchResult, error) {
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
