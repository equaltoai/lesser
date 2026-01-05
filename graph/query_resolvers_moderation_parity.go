package graph

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

func parseSeverityValue(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 2
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 2
	}
	return parsed
}

func actorDomainFromID(actorID, fallbackDomain string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}

	if strings.Contains(actorID, "://") {
		parsedURL, err := url.Parse(actorID)
		if err == nil && parsedURL.Host != "" {
			return parsedURL.Host
		}
		return ""
	}

	parts := strings.Split(actorID, "@")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}

	return fallbackDomain
}

func timePtr(t time.Time) *model.Time {
	if t.IsZero() {
		return nil
	}
	v := model.Time(t)
	return &v
}

// ModerationConsensus covers GET /api/v1/moderation/consensus/{event_id}.
func (r *queryResolver) ModerationConsensus(ctx context.Context, eventID string) (*model.ModerationConsensusResult, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	eventID = strings.TrimSpace(eventID)
	if err := common.ValidateRequiredParam("eventId", eventID); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	moderationRepo := r.Storage.Moderation()
	event, err := moderationRepo.GetModerationEvent(ctx, eventID)
	if err != nil || event == nil {
		return nil, errors.Join(errors.New("moderation event not found"), err)
	}

	reviews, err := moderationRepo.GetModerationReviews(ctx, eventID)
	if err != nil {
		r.Logger.Error("failed to load moderation reviews", zap.String("event_id", eventID), zap.Error(err))
		return nil, errors.Join(errors.New("failed to load moderation reviews"), err)
	}

	var decision *storage.ModerationDecision
	decision, _ = moderationRepo.GetModerationDecision(ctx, event.ObjectID)

	trustRepo := r.Storage.Trust()
	reviewModels := make([]*model.ModerationConsensusReview, 0, len(reviews))
	for _, review := range reviews {
		if review == nil {
			continue
		}

		trustWeight := 0.5
		if trustRepo != nil {
			score, err := trustRepo.GetTrustScore(ctx, review.ReviewerID, string(trust.TrustCategoryContent))
			if err == nil && score != nil {
				trustWeight = score.Score
			}
		}

		reviewerDomain := actorDomainFromID(review.ReviewerID, "")
		reviewModels = append(reviewModels, &model.ModerationConsensusReview{
			ReviewerID:     review.ReviewerID,
			ReviewerDomain: optionalString(reviewerDomain),
			Action:         review.Action,
			Confidence:     review.Confidence,
			TrustWeight:    trustWeight,
			ReviewedAt:     model.Time(review.Created),
		})
	}

	var consensusScore *float64
	var consensusDecision *string
	var decidedAt *model.Time
	if decision != nil {
		consensusScore = &decision.ConsensusScore
		consensusDecision = optionalString(decision.Action)
		decidedAt = timePtr(decision.Decided)
	}

	return &model.ModerationConsensusResult{
		EventID:         event.ID,
		ObjectID:        event.ObjectID,
		Category:        event.Category,
		Severity:        parseSeverityValue(event.Severity),
		ConfidenceScore: event.ConfidenceScore,
		Reviews:         reviewModels,
		ReviewerCount:   len(reviewModels),
		ConsensusScore:  consensusScore,
		Decision:        consensusDecision,
		DecidedAt:       decidedAt,
	}, nil
}

// ModerationHistory covers GET /api/v1/moderation/history/{object_id}.
func (r *queryResolver) ModerationHistory(ctx context.Context, objectID string) (*model.ModerationHistoryResult, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	objectID = strings.TrimSpace(objectID)
	if err := common.ValidateRequiredParam("objectId", objectID); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	history, err := r.Storage.Moderation().GetModerationHistory(ctx, objectID)
	if err != nil || history == nil {
		return nil, errors.Join(errors.New("failed to load moderation history"), err)
	}

	events := make([]*model.ModerationHistoryEvent, 0, len(history.Events))
	for i := range history.Events {
		event := history.Events[i]

		updated := event.Updated
		if updated.IsZero() {
			updated = event.Created
		}

		events = append(events, &model.ModerationHistoryEvent{
			ID:              event.ID,
			EventType:       event.EventType,
			ObjectID:        event.ObjectID,
			ObjectType:      event.ObjectType,
			ActorID:         event.ActorID,
			Category:        event.Category,
			Severity:        parseSeverityValue(event.Severity),
			ConfidenceScore: event.ConfidenceScore,
			Reason:          optionalString(event.Reason),
			CreatedAt:       model.Time(event.Created),
			UpdatedAt:       model.Time(updated),
		})
	}

	decisions := make([]*model.ModerationHistoryDecision, 0, len(history.Decisions))
	for i := range history.Decisions {
		decision := history.Decisions[i]

		decisions = append(decisions, &model.ModerationHistoryDecision{
			ID:               decision.ID,
			EventID:          decision.EventID,
			ObjectID:         decision.ObjectID,
			Action:           decision.Action,
			ConsensusScore:   decision.ConsensusScore,
			ReviewerCount:    decision.ReviewerCount,
			TrustWeightTotal: decision.TrustWeightTotal,
			DecidedAt:        model.Time(decision.Decided),
		})
	}

	timeline := make([]*model.ModerationHistoryTimelineEntry, 0, len(events)+len(decisions))
	for _, event := range events {
		if event == nil {
			continue
		}
		timeline = append(timeline, &model.ModerationHistoryTimelineEntry{
			Timestamp: event.CreatedAt,
			Type:      "event",
			Event:     event,
		})
	}
	for _, decision := range decisions {
		if decision == nil {
			continue
		}
		timeline = append(timeline, &model.ModerationHistoryTimelineEntry{
			Timestamp: decision.DecidedAt,
			Type:      "decision",
			Decision:  decision,
		})
	}

	sort.Slice(timeline, func(i, j int) bool {
		return time.Time(timeline[i].Timestamp).Before(time.Time(timeline[j].Timestamp))
	})

	return &model.ModerationHistoryResult{
		ObjectID:      history.ObjectID,
		Events:        events,
		Decisions:     decisions,
		Timeline:      timeline,
		CurrentStatus: history.CurrentStatus,
		LastUpdated:   model.Time(time.Now()),
	}, nil
}

// ModerationTrustScore covers GET /api/v1/moderation/trust/{actor_id}/score.
func (r *queryResolver) ModerationTrustScore(ctx context.Context, actorID string) (*model.ModerationTrustScore, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	actorID = strings.TrimPrefix(strings.TrimSpace(actorID), "@")
	if err := common.ValidateRequiredParam("actorId", actorID); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Trust() == nil {
		return nil, ErrTrustRepositoryUnavailable
	}

	trustRepo := r.Storage.Trust()

	categories := []trust.TrustCategory{
		trust.TrustCategoryContent,
		trust.TrustCategoryBehavior,
		trust.TrustCategoryTechnical,
		trust.TrustCategoryGeneral,
	}

	scores := make([]*model.TrustCategoryScore, 0, len(categories))
	overallScore := 0.0
	for _, category := range categories {
		score := 0.5

		result, err := trustRepo.GetTrustScore(ctx, actorID, string(category))
		if err == nil && result != nil {
			score = result.Score
		}

		scores = append(scores, &model.TrustCategoryScore{
			Category: string(category),
			Score:    score,
		})
		overallScore += score
	}

	if len(categories) > 0 {
		overallScore /= float64(len(categories))
	}

	trusters, _, err := trustRepo.GetTrustedByRelationships(ctx, actorID, 100, "")
	trusterCount := 0
	if err == nil {
		trusterCount = len(trusters)
	}

	fallbackDomain := ""
	if r.Config != nil {
		fallbackDomain = r.Config.Domain
	}

	return &model.ModerationTrustScore{
		ActorID:      actorID,
		ActorDomain:  optionalString(actorDomainFromID(actorID, fallbackDomain)),
		OverallScore: overallScore,
		Scores:       scores,
		TrusterCount: trusterCount,
		CalculatedAt: model.Time(time.Now()),
	}, nil
}
