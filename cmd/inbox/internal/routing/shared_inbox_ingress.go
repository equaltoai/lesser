package routing

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func (ih *InboxHandler) handleGetSharedInbox(*apptheory.Context) (*apptheory.Response, error) {
	return apptheory.Text(http.StatusMethodNotAllowed, ""), nil
}

func (ih *InboxHandler) handlePostSharedInbox(ctx *apptheory.Context) (*apptheory.Response, error) {
	req, err := ih.initializeSharedInboxRequest(ctx)
	if err != nil {
		return nil, err
	}

	if err := ih.performSecurityChecks(ctx, req); err != nil {
		return nil, err
	}
	if err := ih.verifyAuthentication(ctx, req); err != nil {
		return nil, err
	}
	if err := ih.validateSharedInboxAddressingAndPrivacy(req); err != nil {
		return nil, err
	}
	if err := ih.storeAndProcessActivity(ctx, req); err != nil {
		return nil, err
	}

	ih.recordSuccessAndComplete(ctx, req)
	return apptheory.Text(http.StatusAccepted, ""), nil
}

func (ih *InboxHandler) initializeActorInboxRequest(ctx *apptheory.Context) (*InboxRequest, error) {
	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, errors.ValidationFailed("username", "missing username parameter")
	}

	if err := ih.rejectLockedBootstrapActor(ctx.Context(), username); err != nil {
		return nil, err
	}

	ih.logger.Info("received inbox POST request",
		zap.String("username", username),
		zap.String("content_type", headerValue(ctx, "Content-Type")),
		zap.String("user_agent", headerValue(ctx, "User-Agent")),
		zap.String("request_id", ctx.RequestID))

	activity, body, err := ih.parseAndValidateInboundActivity(ctx)
	if err != nil {
		return nil, err
	}

	actor, err := ih.actorRepository.GetActorByUsername(ctx.Context(), username)
	if err != nil {
		if err.Error() == "actor not found" {
			return nil, errors.NotFound("actor")
		}
		ih.logger.Error("failed to get actor", zap.Error(err))
		return nil, errors.InternalWithCause(err, "internal server error")
	}

	if err := ih.validateResolvedTargetActors(activity, []*activitypub.Actor{actor}, true); err != nil {
		return nil, err
	}

	return ih.buildInboxRequest(username, activity, []*activitypub.Actor{actor}, body), nil
}

func (ih *InboxHandler) initializeSharedInboxRequest(ctx *apptheory.Context) (*InboxRequest, error) {
	ih.logger.Info("received shared inbox POST request",
		zap.String("content_type", headerValue(ctx, "Content-Type")),
		zap.String("user_agent", headerValue(ctx, "User-Agent")),
		zap.String("request_id", ctx.RequestID))

	activity, body, err := ih.parseAndValidateInboundActivity(ctx)
	if err != nil {
		return nil, err
	}

	resolver := sharedInboxTargetResolver{
		actorRepository:        ih.actorRepository,
		relationshipRepository: ih.relationshipRepository,
		activityRepository:     ih.activityRepository,
		objectRepository:       ih.objectRepository,
		localDomain:            ih.getConfig().Domain,
	}
	targetActors, err := resolver.Resolve(ctx.Context(), activity)
	if err != nil {
		return nil, errors.InternalWithCause(err, "failed to resolve shared inbox targets")
	}

	targetActors, err = ih.filterBootstrapTargets(ctx.Context(), targetActors)
	if err != nil {
		return nil, err
	}
	if len(targetActors) == 0 {
		return nil, errors.NotFound("resource")
	}

	if err := ih.validateResolvedTargetActors(activity, targetActors, false); err != nil {
		return nil, err
	}

	return ih.buildInboxRequest(targetActors[0].PreferredUsername, activity, targetActors, body), nil
}

func (ih *InboxHandler) parseAndValidateInboundActivity(ctx *apptheory.Context) (*activitypub.Activity, []byte, error) {
	contentType := headerValue(ctx, "Content-Type")
	if err := common.ValidateActivityPubContentType(contentType); err != nil {
		ih.logger.Warn("invalid content type", zap.String("content_type", contentType), zap.Error(err))
		return nil, nil, errors.ValidationFailed("Content-Type", fmt.Sprintf("invalid Content-Type: %v", err))
	}

	body := ctx.Request.Body
	if err := ih.validateRequestBody(body); err != nil {
		return nil, nil, err
	}

	activity, err := ih.parseActivity(body)
	if err != nil {
		return nil, nil, err
	}

	if err := ih.validateActivityStructure(activity); err != nil {
		return nil, nil, err
	}

	return activity, body, nil
}

func (ih *InboxHandler) validateActivityStructure(activity *activitypub.Activity) error {
	if err := ih.validateBasicActivity(activity); err != nil {
		return err
	}
	if err := ih.validateActivityAddressing(activity); err != nil {
		return err
	}
	if err := ih.validateActorUsername(activity.Actor); err != nil {
		return err
	}
	if err := ih.validateCreateActivityObject(activity); err != nil {
		return err
	}
	if err := ih.validateComprehensiveAddressing(activity); err != nil {
		return err
	}
	return nil
}

func (ih *InboxHandler) validateResolvedTargetActors(activity *activitypub.Activity, actors []*activitypub.Actor, requireTargeting bool) error {
	for _, actor := range actors {
		if actor == nil {
			continue
		}
		if err := ih.validateBasicActor(actor); err != nil {
			return err
		}
		if err := ih.validateActorPublicKey(actor); err != nil {
			return err
		}
		if requireTargeting {
			if err := ih.validateActivityTargeting(activity, actor); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ih *InboxHandler) buildInboxRequest(username string, activity *activitypub.Activity, targetActors []*activitypub.Actor, body []byte) *InboxRequest {
	startTime := time.Now()
	actorDomain := ih.extractDomainFromURL(activity.Actor)
	var primaryActor *activitypub.Actor
	if len(targetActors) > 0 {
		primaryActor = targetActors[0]
	}
	readCount := int64(len(targetActors))
	if readCount == 0 {
		readCount = 1
	}
	return &InboxRequest{
		Username:     username,
		Activity:     activity,
		Actor:        primaryActor,
		TargetActors: targetActors,
		Body:         body,
		ActorDomain:  actorDomain,
		StartTime:    startTime,
		CostParams: &federation.CostCalculationParams{
			ActivityID:         activity.ID,
			Domain:             actorDomain,
			ActivityType:       activity.Type,
			Direction:          "inbound",
			OperationType:      "inbox_processing",
			Timestamp:          startTime,
			PayloadSize:        int64(len(body)),
			LambdaMemoryMB:     512,
			DynamoDBReadCount:  readCount,
			DynamoDBWriteCount: 0,
		},
	}
}

func (ih *InboxHandler) validateActorInboxAddressingAndPrivacy(req *InboxRequest) error {
	start := time.Now()
	addressingValidator := activitypub.NewAddressingValidator()
	if err := ih.validateAddressingFields(req, addressingValidator, start); err != nil {
		return err
	}

	if !ih.isAddressedTo(req.Activity, req.Actor) {
		ih.logger.Warn("activity not addressed to this actor",
			zap.String("actor", req.Activity.Actor),
			zap.String("target_actor", req.Actor.ID),
			zap.String("activity_id", req.Activity.ID))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, "Activity not addressed to target actor", 1)
		return errors.NotFound("resource")
	}

	if addressingValidator.IsDirectMessage(req.Activity) {
		if err := ih.validateDirectMessage(req.Activity, req.Actor); err != nil {
			ih.logger.Warn("direct message validation failed",
				zap.String("actor", req.Activity.Actor),
				zap.String("activity_id", req.Activity.ID),
				zap.Error(err))
			req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
			ih.recordFailureCost(req, fmt.Sprintf("Direct message validation failed: %v", err), 1)
			return errors.ValidationFailed("direct_message", "direct message validation failed").WithInternalError(err)
		}
	}

	req.CostParams.ProcessingTimeMs += time.Since(start).Milliseconds()
	ih.logger.Debug("addressing and privacy validation successful",
		zap.String("actor", req.Activity.Actor),
		zap.String("visibility", addressingValidator.GetVisibilityLevel(req.Activity)))
	return nil
}

func (ih *InboxHandler) validateSharedInboxAddressingAndPrivacy(req *InboxRequest) error {
	start := time.Now()
	addressingValidator := activitypub.NewAddressingValidator()
	if err := ih.validateAddressingFields(req, addressingValidator, start); err != nil {
		return err
	}

	if len(req.TargetActors) == 0 {
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, "Shared inbox activity did not resolve to a local target", 1)
		return errors.NotFound("resource")
	}

	if addressingValidator.IsDirectMessage(req.Activity) {
		if err := ih.validateDirectMessage(req.Activity, req.Actor); err != nil {
			ih.logger.Warn("shared inbox direct message validation failed",
				zap.String("actor", req.Activity.Actor),
				zap.String("activity_id", req.Activity.ID),
				zap.Error(err))
			req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
			ih.recordFailureCost(req, fmt.Sprintf("Direct message validation failed: %v", err), 1)
			return errors.ValidationFailed("direct_message", "direct message validation failed").WithInternalError(err)
		}
	}

	req.CostParams.ProcessingTimeMs += time.Since(start).Milliseconds()
	ih.logger.Debug("shared inbox addressing and privacy validation successful",
		zap.String("actor", req.Activity.Actor),
		zap.Int("target_count", len(req.TargetActors)),
		zap.String("visibility", addressingValidator.GetVisibilityLevel(req.Activity)))
	return nil
}

func (ih *InboxHandler) validateAddressingFields(req *InboxRequest, addressingValidator *activitypub.AddressingValidator, start time.Time) error {
	if err := addressingValidator.ValidateAddressing(req.Activity); err != nil {
		ih.logger.Warn("activity has invalid addressing fields",
			zap.String("actor", req.Activity.Actor),
			zap.String("activity_id", req.Activity.ID),
			zap.Error(err))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Invalid addressing: %v", err), 1)
		return errors.ValidationFailed("addressing", "invalid addressing fields").WithInternalError(err)
	}

	if err := addressingValidator.ValidatePrivacyCompliance(req.Activity); err != nil {
		ih.logger.Warn("activity violates privacy compliance",
			zap.String("actor", req.Activity.Actor),
			zap.String("activity_id", req.Activity.ID),
			zap.Error(err))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Privacy violation: %v", err), 1)
		return errors.ValidationFailed("privacy", "privacy compliance violation").WithInternalError(err)
	}

	return nil
}

func (ih *InboxHandler) rejectLockedBootstrapActor(ctx context.Context, username string) error {
	if ih.instanceRepository == nil {
		return nil
	}

	state, err := ih.instanceRepository.GetInstanceState(ctx)
	bootstrapUsername := models.DefaultBootstrapUsername
	if err == nil && strings.TrimSpace(state.BootstrapUsername) != "" {
		bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
	}

	if (err != nil && strings.EqualFold(username, bootstrapUsername)) ||
		(err == nil && state.Locked && strings.EqualFold(username, bootstrapUsername)) {
		return errors.Forbidden("bootstrap actor does not accept federation while instance is locked")
	}

	return nil
}

func (ih *InboxHandler) filterBootstrapTargets(ctx context.Context, actors []*activitypub.Actor) ([]*activitypub.Actor, error) {
	if ih.instanceRepository == nil || len(actors) == 0 {
		return actors, nil
	}

	state, err := ih.instanceRepository.GetInstanceState(ctx)
	bootstrapUsername := models.DefaultBootstrapUsername
	locked := false
	if err == nil {
		locked = state.Locked
		if strings.TrimSpace(state.BootstrapUsername) != "" {
			bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
		}
	}

	filtered := make([]*activitypub.Actor, 0, len(actors))
	for _, actor := range actors {
		if actor == nil {
			continue
		}
		if strings.EqualFold(actor.PreferredUsername, bootstrapUsername) && (locked || err != nil) {
			continue
		}
		filtered = append(filtered, actor)
	}

	return filtered, nil
}
