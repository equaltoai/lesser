package graph

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// SubmitModerationReview covers POST /api/v1/moderation/review.
func (r *mutationResolver) SubmitModerationReview(ctx context.Context, input model.ModerationReviewInput) (*model.ModerationReviewResult, error) {
	username, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	eventID := strings.TrimSpace(input.EventID)
	if err := common.ValidateRequiredParam("eventId", eventID); err != nil {
		return nil, err
	}

	if err := common.ValidateFloatRange("confidence", input.Confidence, 0, 1); err != nil {
		return nil, err
	}

	if input.Severity < 1 || input.Severity > 4 {
		return nil, common.ValidationError{
			Field:   "severity",
			Message: "must be between 1 and 4",
		}
	}

	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	actorID := username
	if r.Storage.Actor() != nil {
		actor, err := r.Storage.Actor().GetActor(ctx, username)
		if err == nil && actor != nil && actor.ID != "" {
			actorID = actor.ID
		}
	}

	now := time.Now()
	reviewID := fmt.Sprintf("review-%s-%s-%d", eventID, username, now.Unix())

	note := ""
	if input.Notes != nil {
		note = strings.TrimSpace(*input.Notes)
	}

	storageReview := &storage.ModerationReview{
		ID:          reviewID,
		EventID:     eventID,
		ReviewerID:  actorID,
		ReviewerRep: 0.5,
		Action:      strings.TrimSpace(input.Action),
		Severity:    strconv.Itoa(input.Severity),
		Note:        note,
		Tags:        []string{},
		Metadata:    map[string]interface{}{},
		Confidence:  input.Confidence,
		Created:     now,
	}

	if err := r.Storage.Moderation().AddModerationReview(ctx, storageReview); err != nil {
		r.Logger.Error("failed to add moderation review", zap.Error(err))
		return nil, errors.Join(errors.New("failed to add moderation review"), err)
	}

	return &model.ModerationReviewResult{
		ReviewID:   storageReview.ID,
		EventID:    storageReview.EventID,
		Action:     storageReview.Action,
		ReviewedAt: model.Time(storageReview.Created),
	}, nil
}
