package graph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	dbmodels "github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	filterActionWarn = "warn"
	filterActionHide = "hide"
	filterActionBlur = "blur"

	defaultFilterSeverity = "medium"
)

func (r *mutationResolver) CreateFilter(ctx context.Context, input model.CreateFilterInput) (*mastodon.Filter, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateFilterTitle(input.Title); err != nil {
		return nil, err
	}
	if err := common.ValidateFilterContext(input.Context); err != nil {
		return nil, err
	}

	action := filterActionWarn
	if input.FilterAction != nil {
		switch *input.FilterAction {
		case model.FilterActionWarn:
			action = filterActionWarn
		case model.FilterActionHide:
			action = filterActionHide
		case model.FilterActionBlur:
			action = filterActionBlur
		default:
			return nil, fmt.Errorf("invalid filter action: %q", *input.FilterAction)
		}
	}

	if err := common.ValidateFilterAction(action); err != nil {
		return nil, err
	}

	var expiresAt *time.Time
	if input.ExpiresInSeconds != nil {
		if *input.ExpiresInSeconds > 0 {
			t := time.Now().Add(time.Duration(*input.ExpiresInSeconds) * time.Second)
			expiresAt = &t
		}
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return nil, errors.New("moderation repository is not available")
	}

	filter := &storage.Filter{
		Username:     username,
		Title:        input.Title,
		Context:      input.Context,
		FilterAction: action,
		ExpiresAt:    expiresAt,
	}

	if err := moderationRepo.CreateFilter(ctx, filter); err != nil {
		r.Logger.Error("failed to create filter", zap.String("user", username), zap.Error(err))
		return nil, errors.Join(errors.New("failed to create filter"), err)
	}

	// Add keywords if provided
	for _, kwInput := range input.Keywords {
		if kwInput == nil {
			continue
		}

		keyword := strings.TrimSpace(kwInput.Keyword)
		if err := common.ValidateFilterKeyword(keyword); err != nil {
			return nil, err
		}

		wholeWord := false
		if kwInput.WholeWord != nil {
			wholeWord = *kwInput.WholeWord
		}

		kw := &storage.FilterKeyword{
			Keyword:   keyword,
			WholeWord: wholeWord,
		}
		if err := moderationRepo.AddFilterKeyword(ctx, filter.ID, kw); err != nil {
			r.Logger.Error("failed to add filter keyword",
				zap.String("filter_id", filter.ID),
				zap.Error(err))
			return nil, errors.Join(errors.New("failed to add filter keyword"), err)
		}
	}

	keywords, _ := moderationRepo.GetFilterKeywords(ctx, filter.ID)
	statuses, _ := moderationRepo.GetFilterStatuses(ctx, filter.ID)

	converter := r.MastodonConv
	if converter == nil {
		baseURL := ""
		if r.Config != nil {
			baseURL = r.Config.BaseURL()
		}
		converter = mastodon.NewConverter(baseURL)
	}

	return converter.ConvertFilterToMastodon(filter, keywords, statuses), nil
}

func (r *mutationResolver) UpdateFilter(ctx context.Context, id string, input model.UpdateFilterInput) (*mastodon.Filter, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateFilterParamID(id); err != nil {
		return nil, err
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return nil, errors.New("moderation repository is not available")
	}

	filter, err := moderationRepo.GetFilter(ctx, id)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get filter"), err)
	}
	if filter == nil || filter.Username != username {
		return nil, common.ErrNotFound("filter")
	}

	updates := make(map[string]any)
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if err := common.ValidateFilterTitle(title); err != nil {
			return nil, err
		}
		updates["title"] = title
	}
	if input.Context != nil {
		if err := common.ValidateFilterContext(input.Context); err != nil {
			return nil, err
		}
		updates["context"] = input.Context
	}
	if input.FilterAction != nil {
		var action string
		switch *input.FilterAction {
		case model.FilterActionWarn:
			action = filterActionWarn
		case model.FilterActionHide:
			action = filterActionHide
		case model.FilterActionBlur:
			action = filterActionBlur
		default:
			return nil, fmt.Errorf("invalid filter action: %q", *input.FilterAction)
		}
		if err := common.ValidateFilterAction(action); err != nil {
			return nil, err
		}
		updates["filter_action"] = action
	}
	if input.ExpiresInSeconds != nil {
		if *input.ExpiresInSeconds > 0 {
			expires := time.Now().Add(time.Duration(*input.ExpiresInSeconds) * time.Second)
			updates["expires_at"] = &expires
		} else {
			var noExpiry *time.Time
			updates["expires_at"] = noExpiry
		}
	}

	if err := moderationRepo.UpdateFilter(ctx, id, updates); err != nil {
		r.Logger.Error("failed to update filter",
			zap.String("user", username),
			zap.String("filter_id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update filter"), err)
	}

	updated, err := moderationRepo.GetFilter(ctx, id)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get updated filter"), err)
	}
	keywords, _ := moderationRepo.GetFilterKeywords(ctx, id)
	statuses, _ := moderationRepo.GetFilterStatuses(ctx, id)

	converter := r.MastodonConv
	if converter == nil {
		baseURL := ""
		if r.Config != nil {
			baseURL = r.Config.BaseURL()
		}
		converter = mastodon.NewConverter(baseURL)
	}

	return converter.ConvertFilterToMastodon(updated, keywords, statuses), nil
}

func (r *mutationResolver) DeleteFilter(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := common.ValidateFilterParamID(id); err != nil {
		return false, err
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return false, errors.New("moderation repository is not available")
	}

	filter, err := moderationRepo.GetFilter(ctx, id)
	if err != nil {
		return false, errors.Join(errors.New("failed to get filter"), err)
	}
	if filter == nil || filter.Username != username {
		return false, common.ErrNotFound("filter")
	}

	if err := moderationRepo.DeleteFilter(ctx, id); err != nil {
		r.Logger.Error("failed to delete filter",
			zap.String("user", username),
			zap.String("filter_id", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to delete filter"), err)
	}

	return true, nil
}

func (r *mutationResolver) AddFilterKeyword(ctx context.Context, filterID string, input model.AddFilterKeywordInput) (*mastodon.FilterKeyword, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return nil, err
	}

	keyword := strings.TrimSpace(input.Keyword)
	if err := common.ValidateFilterKeyword(keyword); err != nil {
		return nil, err
	}

	wholeWord := false
	if input.WholeWord != nil {
		wholeWord = *input.WholeWord
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return nil, errors.New("moderation repository is not available")
	}

	filter, err := moderationRepo.GetFilter(ctx, filterID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get filter"), err)
	}
	if filter == nil || filter.Username != username {
		return nil, common.ErrNotFound("filter")
	}

	kw := &storage.FilterKeyword{
		Keyword:   keyword,
		WholeWord: wholeWord,
	}
	if err := moderationRepo.AddFilterKeyword(ctx, filterID, kw); err != nil {
		return nil, errors.Join(errors.New("failed to add filter keyword"), err)
	}

	return &mastodon.FilterKeyword{
		ID:        kw.ID,
		Keyword:   kw.Keyword,
		WholeWord: kw.WholeWord,
	}, nil
}

func (r *mutationResolver) DeleteFilterKeyword(ctx context.Context, filterID string, keywordID string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return false, err
	}
	if err := common.ValidateFilterParamID(keywordID); err != nil {
		return false, err
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return false, errors.New("moderation repository is not available")
	}

	filter, err := moderationRepo.GetFilter(ctx, filterID)
	if err != nil {
		return false, errors.Join(errors.New("failed to get filter"), err)
	}
	if filter == nil || filter.Username != username {
		return false, common.ErrNotFound("filter")
	}

	if err := moderationRepo.DeleteFilterKeyword(ctx, filterID, keywordID); err != nil {
		return false, errors.Join(errors.New("failed to delete filter keyword"), err)
	}

	return true, nil
}

func (r *mutationResolver) AddFilterStatus(ctx context.Context, filterID string, statusID string) (*mastodon.FilterStatus, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("statusId", strings.TrimSpace(statusID)); err != nil {
		return nil, err
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return nil, errors.New("moderation repository is not available")
	}

	filter, err := moderationRepo.GetFilter(ctx, filterID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get filter"), err)
	}
	if filter == nil || filter.Username != username {
		return nil, common.ErrNotFound("filter")
	}

	fs := &storage.FilterStatus{
		StatusID: strings.TrimSpace(statusID),
	}
	if err := moderationRepo.AddFilterStatus(ctx, filterID, fs); err != nil {
		return nil, errors.Join(errors.New("failed to add filter status"), err)
	}

	return &mastodon.FilterStatus{
		ID:       fs.ID,
		StatusID: fs.StatusID,
	}, nil
}

func (r *mutationResolver) DeleteFilterStatus(ctx context.Context, filterID string, filterStatusID string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return false, err
	}
	if err := common.ValidateRequiredParam("filterStatusId", strings.TrimSpace(filterStatusID)); err != nil {
		return false, err
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return false, errors.New("moderation repository is not available")
	}

	filter, err := moderationRepo.GetFilter(ctx, filterID)
	if err != nil {
		return false, errors.Join(errors.New("failed to get filter"), err)
	}
	if filter == nil || filter.Username != username {
		return false, common.ErrNotFound("filter")
	}

	statuses, err := moderationRepo.GetFilterStatuses(ctx, filterID)
	if err != nil {
		return false, errors.Join(errors.New("failed to get filter statuses"), err)
	}

	targetStatusID := ""
	for _, st := range statuses {
		if st == nil {
			continue
		}
		if st.ID == filterStatusID || st.StatusID == filterStatusID {
			targetStatusID = st.StatusID
			break
		}
	}
	if targetStatusID == "" {
		return false, common.ErrNotFound("filter status")
	}

	if err := moderationRepo.DeleteFilterStatus(ctx, filterID, targetStatusID); err != nil {
		return false, errors.Join(errors.New("failed to delete filter status"), err)
	}

	return true, nil
}

func (r *mutationResolver) TestFilters(ctx context.Context, input model.FilterTestInput) (*model.FilterTestPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(input.Content)
	if err := common.ValidateRequiredParam("content", content); err != nil {
		return nil, err
	}
	if err := common.ValidateSliceNotEmpty("context", input.Context); err != nil {
		return nil, err
	}

	filters, keywordRepo, err := r.resolveFilterModelsForTest(ctx, username)
	if err != nil {
		return nil, err
	}

	engine := moderation.NewAdvancedFilterEngine(r.Logger)
	if keywordRepo != nil {
		engine.SetFilterRepository(keywordRepo)
	}

	merged, err := evaluateFilterTest(ctx, engine, content, filters, input.Context)
	if err != nil {
		return nil, err
	}

	results := buildFilterTestResults(merged)

	return &model.FilterTestPayload{
		Content:      content,
		TotalFilters: len(filters),
		MatchedCount: len(results),
		Results:      results,
	}, nil
}

func (r *mutationResolver) resolveFilterModelsForTest(ctx context.Context, username string) ([]*dbmodels.Filter, moderation.FilterRepository, error) {
	filterRepo := r.Storage.Filter()
	if filterRepo != nil {
		filters, err := filterRepo.GetUserFilters(ctx, username)
		if err != nil {
			r.Logger.Error("failed to get user filters", zap.String("user", username), zap.Error(err))
			return nil, nil, errors.Join(errors.New("failed to get user filters"), err)
		}
		return filters, filterRepo, nil
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return nil, nil, errors.New("filter repositories are not available")
	}

	storageFilters, err := moderationRepo.GetFiltersForUser(ctx, username)
	if err != nil {
		return nil, nil, errors.Join(errors.New("failed to get user filters"), err)
	}

	filters := make([]*dbmodels.Filter, 0, len(storageFilters))
	for _, sf := range storageFilters {
		if sf == nil {
			continue
		}
		filters = append(filters, &dbmodels.Filter{
			ID:           sf.ID,
			Username:     sf.Username,
			Title:        sf.Title,
			Context:      sf.Context,
			FilterAction: sf.FilterAction,
			ExpiresAt:    sf.ExpiresAt,
			CreatedAt:    sf.CreatedAt,
			UpdatedAt:    sf.UpdatedAt,
		})
	}

	return filters, nil, nil
}

func evaluateFilterTest(
	ctx context.Context,
	engine *moderation.AdvancedFilterEngine,
	content string,
	filters []*dbmodels.Filter,
	contexts []string,
) (map[string]*moderation.FilterResult, error) {
	if engine == nil {
		return nil, errors.New("filter engine is not available")
	}

	merged := make(map[string]*moderation.FilterResult)
	now := time.Now()
	for _, ctxType := range contexts {
		ctxType = strings.TrimSpace(ctxType)
		if ctxType == "" {
			continue
		}

		contentCtx := &moderation.ContentContext{
			Type:       ctxType,
			Timestamp:  now,
			IsReply:    false,
			HasMedia:   false,
			Language:   "en",
			Visibility: "public",
		}

		results, err := engine.EvaluateContent(ctx, content, filters, contentCtx)
		if err != nil {
			return nil, errors.Join(errors.New("failed to evaluate content"), err)
		}

		for _, res := range results {
			if res == nil || !res.Matched || res.Filter == nil {
				continue
			}
			upsertMergedFilterResult(merged, res)
		}
	}

	return merged, nil
}

func upsertMergedFilterResult(merged map[string]*moderation.FilterResult, incoming *moderation.FilterResult) {
	if merged == nil || incoming == nil || incoming.Filter == nil {
		return
	}

	existing, ok := merged[incoming.Filter.ID]
	if !ok {
		merged[incoming.Filter.ID] = incoming
		return
	}

	if incoming.MatchScore > existing.MatchScore {
		existing.MatchScore = incoming.MatchScore
	}

	seen := make(map[string]struct{}, len(existing.MatchedRules))
	for _, rule := range existing.MatchedRules {
		seen[rule] = struct{}{}
	}
	for _, rule := range incoming.MatchedRules {
		if _, exists := seen[rule]; exists {
			continue
		}
		existing.MatchedRules = append(existing.MatchedRules, rule)
	}
}

func buildFilterTestResults(merged map[string]*moderation.FilterResult) []*model.FilterTestResult {
	results := make([]*model.FilterTestResult, 0, len(merged))
	for _, res := range merged {
		if res == nil || res.Filter == nil {
			continue
		}

		severity := res.Severity
		if severity == "" {
			severity = defaultFilterSeverity
		}

		matchedRules := res.MatchedRules
		if matchedRules == nil {
			matchedRules = []string{}
		}

		results = append(results, &model.FilterTestResult{
			Action:       res.Action,
			Severity:     severity,
			MatchScore:   res.MatchScore,
			MatchedRules: matchedRules,
			FilterID:     res.Filter.ID,
			FilterTitle:  res.Filter.Title,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i] == nil {
			return false
		}
		if results[j] == nil {
			return true
		}
		return results[i].MatchScore > results[j].MatchScore
	})

	return results
}
