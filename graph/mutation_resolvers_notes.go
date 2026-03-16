package graph

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ====================================================================
// MUTATION RESOLVERS - CORE STATUS OPERATIONS
// ====================================================================

// CreateNote is the resolver for the createNote field.
func (r *mutationResolver) CreateNote(ctx context.Context, input model.CreateNoteInput) (*model.CreateNotePayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	cmd, quoteTargetID, err := r.buildCreateNoteCommand(username, input)
	if err != nil {
		return nil, err
	}

	if claims, ok := ctx.Value(common.ContextKeyClaims).(*auth.Claims); ok && claims != nil && claims.IsAgent {
		attribution, err := r.buildAgentPostAttribution(ctx, claims, input.AgentAttribution)
		if err != nil {
			return nil, err
		}
		cmd.AgentAttribution = attribution
	}

	// Handle mentions and tags
	// These would be parsed from content in the service

	var requestLogger *zap.Logger
	if r.Logger != nil {
		requestLogger = r.Logger
		requestLogger.Info("createNote resolver started",
			zap.String("user", username),
			zap.Int("content_length", len(input.Content)))
	}

	// Create note using service
	serviceStart := time.Now()
	result, err := r.Registry.Notes().CreateNote(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to create note",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create note"), err)
	}

	if quoteTargetID != "" && result != nil && result.Note != nil {
		if err := r.attachQuoteToTarget(ctx, username, quoteTargetID, result.Note); err != nil {
			return nil, err
		}
	}

	if requestLogger != nil && result != nil && result.Note != nil {
		requestLogger.Info("createNote service completed",
			zap.String("user", username),
			zap.String("status_id", result.Note.StatusID),
			zap.Duration("duration", time.Since(serviceStart)))
	}

	// Track costs
	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	if err := common.ValidateSliceNotEmpty("media_ids", cmd.MediaIDs); err == nil {
		// Track S3 cost using centralized tracker
		r.trackS3Operation(ctx, "get", len(cmd.MediaIDs))
	}

	// Build response
	now := time.Now()
	convertStart := time.Now()
	objectModel := r.convertStatusToObject(ctx, result.Note)
	if requestLogger != nil && result != nil && result.Note != nil {
		requestLogger.Info("createNote object conversion completed",
			zap.String("status_id", result.Note.StatusID),
			zap.Duration("duration", time.Since(convertStart)),
			zap.Bool("actor_loaded", objectModel != nil && objectModel.Actor != nil))
	}

	payload := &model.CreateNotePayload{
		Object: objectModel,
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:        generateID(),
				Type:      activitypub.CreateType,
				Published: &now,
			},
			Actor:  username,
			Object: result.Note,
		},
		Cost: &model.CostUpdate{
			OperationCost:     100,  // 100 microcents = $0.001 for object creation
			DailyTotal:        0.10, // Estimated daily total
			MonthlyProjection: 3.00, // Estimated monthly projection
		},
	}

	if requestLogger != nil && result != nil && result.Note != nil {
		requestLogger.Info("createNote resolver completed",
			zap.String("status_id", result.Note.StatusID),
			zap.Duration("total_duration", time.Since(serviceStart)))
	}

	return payload, nil
}

const (
	agentAttributionMaxMemoryCitations = 20
	agentAttributionMaxTriggerDetails  = 500

	agentAttributionDefaultTriggerType = "manual"
	agentAttributionUnknownModelID     = "unknown"
)

var allowedAgentAttributionTriggerTypes = map[string]struct{}{
	"scheduled":     {},
	"mention":       {},
	"hashtag_watch": {},
	"manual":        {},
}

func (r *mutationResolver) buildAgentPostAttribution(ctx context.Context, claims *auth.Claims, input *model.AgentPostAttributionInput) (*activitypub.AgentPostAttribution, error) {
	if claims == nil || !claims.IsAgent {
		return nil, nil
	}

	if r.Storage == nil || r.Storage.User() == nil {
		return nil, ErrStorageUnavailable
	}

	agentUser, _ := r.Storage.User().GetUser(ctx, claims.Username)
	if agentUser == nil || !agentUser.IsAgent {
		return nil, apperrors.NewAuthError(apperrors.CodeForbidden, "agent attribution is only available for agent accounts")
	}

	triggerType := agentAttributionDefaultTriggerType
	triggerDetails := ""
	var memoryCitations []string

	if input != nil {
		if t := strings.ToLower(strings.TrimSpace(derefString(input.TriggerType))); t != "" {
			triggerType = t
		}

		triggerDetails = strings.TrimSpace(derefString(input.TriggerDetails))
		if triggerDetails != "" {
			if err := common.ValidateStringLength("trigger_details", triggerDetails, 0, agentAttributionMaxTriggerDetails); err != nil {
				return nil, common.ValidationError{Field: "agentAttribution.triggerDetails", Message: "is too long"}
			}
		}

		memoryCitations = normalizeMemoryCitations(input.MemoryCitations)
		if len(memoryCitations) > agentAttributionMaxMemoryCitations {
			return nil, common.ValidationError{Field: "agentAttribution.memoryCitations", Message: "has too many entries"}
		}
		for _, id := range memoryCitations {
			if err := common.ValidateStatusID(id); err != nil {
				return nil, common.ValidationError{Field: "agentAttribution.memoryCitations", Message: "contains an invalid status id"}
			}
		}
	}

	if _, ok := allowedAgentAttributionTriggerTypes[triggerType]; !ok {
		return nil, common.ValidationError{Field: "agentAttribution.triggerType", Message: "must be one of: scheduled, mention, hashtag_watch, manual"}
	}

	delegatedBy := strings.TrimSpace(claims.DelegatedBy)
	if delegatedBy == "" {
		delegatedBy = strings.TrimSpace(agentUser.AgentOwner)
	}
	delegatedBy = r.normalizeDelegatedByActorURI(delegatedBy)

	modelID := strings.TrimSpace(agentUser.AgentVersion)
	if modelID == "" {
		modelID = agentAttributionUnknownModelID
	}

	return &activitypub.AgentPostAttribution{
		TriggerType:     triggerType,
		TriggerDetails:  triggerDetails,
		MemoryCitations: memoryCitations,
		DelegatedBy:     delegatedBy,
		Scopes:          append([]string(nil), claims.Scopes...),
		Constraints:     buildAgentCapabilityConstraints(agentUser.AgentCapabilities),
		SchemaVersion:   activitypub.AgentAttributionSchemaVersion,
		ModelID:         modelID,
	}, nil
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

func (r *mutationResolver) attachQuoteToTarget(ctx context.Context, username, targetStatusID string, quoteStatus *models.Status) error {
	if quoteStatus == nil {
		return errors.New("quote status not available for attachment")
	}

	quotesService := r.Registry.Quotes()
	if quotesService == nil {
		r.Logger.Error("quotes service not available for quote attachment")
		return errors.New("quotes service is not available")
	}

	_, err := quotesService.AttachQuoteToStatus(ctx, quoteStatus, targetStatusID)
	if err != nil {
		r.Logger.Error("failed to attach quote relationship",
			zap.String("user", username),
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return err
	}

	r.trackDynamoOperation(ctx, "write", 1)

	if r.Logger != nil {
		r.Logger.Info("attached quote relationship",
			zap.String("user", username),
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.String("target_status_id", targetStatusID))
	}

	return nil
}

func (r *mutationResolver) buildCreateNoteCommand(username string, input model.CreateNoteInput) (*notes.CreateNoteCommand, string, error) {
	if err := common.ValidateContentOrAttachments(input.Content, input.AttachmentIds); err != nil {
		return nil, "", err
	}

	quoteTargetID := ""
	if input.QuoteID != nil {
		quoteTargetID = strings.TrimSpace(*input.QuoteID)
	}

	cmd := &notes.CreateNoteCommand{
		AuthorID:   username,
		Content:    input.Content,
		Visibility: strings.ToLower(input.Visibility.String()),
		Sensitive:  input.Sensitive != nil && *input.Sensitive,
	}

	if input.SpoilerText != nil {
		cmd.SpoilerText = *input.SpoilerText
	}
	if input.InReplyToID != nil {
		cmd.InReplyToID = *input.InReplyToID
	}
	if input.AttachmentIds != nil {
		cmd.MediaIDs = input.AttachmentIds
	}

	if input.Poll != nil {
		if err := r.applyPollInput(cmd, input.Poll); err != nil {
			return nil, "", err
		}
	}

	return cmd, quoteTargetID, nil
}

func (r *mutationResolver) applyPollInput(cmd *notes.CreateNoteCommand, pollInput *model.PollParamsInput) error {
	pollOptionsAny := make([]any, len(pollInput.Options))
	for i, option := range pollInput.Options {
		pollOptionsAny[i] = option
	}

	pollParams := map[string]any{
		"options":    pollOptionsAny,
		"expires_in": float64(pollInput.ExpiresIn),
	}

	if pollInput.Multiple != nil {
		pollParams["multiple"] = *pollInput.Multiple
	}
	if pollInput.HideTotals != nil {
		pollParams["hide_totals"] = *pollInput.HideTotals
	}

	if err := common.ValidatePollParams(pollParams); err != nil {
		return err
	}

	cmd.PollOptions = pollInput.Options
	cmd.PollExpiresIn = pollInput.ExpiresIn
	cmd.PollMultiple = pollInput.Multiple != nil && *pollInput.Multiple
	cmd.PollHideTotals = pollInput.HideTotals != nil && *pollInput.HideTotals
	return nil
}

// DeleteObject is the resolver for the deleteObject field.
func (r *mutationResolver) DeleteObject(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	// Delete using notes service
	err = r.Registry.Notes().DeleteNote(ctx, &notes.DeleteNoteCommand{
		StatusID:  id,
		DeleterID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to delete object",
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to delete object"), err)
	}

	// Track costs
	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	return true, nil
}

// ScheduleStatus is the resolver for the scheduleStatus field.
func (r *mutationResolver) ScheduleStatus(ctx context.Context, input model.ScheduleStatusInput) (*model.ScheduledStatus, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	scheduledTime := time.Time(input.ScheduledAt)
	if scheduledTime.Before(time.Now().Add(5 * time.Minute)) {
		return nil, ErrScheduledTimeMinimum
	}
	if scheduledTime.After(time.Now().AddDate(1, 0, 0)) {
		return nil, ErrScheduledTimeMaximum
	}

	cmd := &scheduled.CreateScheduledStatusCommand{
		Username:    username,
		Status:      input.Text,
		ScheduledAt: scheduledTime,
	}

	if input.Visibility != nil {
		cmd.Visibility = string(*input.Visibility)
	} else {
		cmd.Visibility = StreamNamePublic
	}

	if input.Sensitive != nil {
		cmd.Sensitive = *input.Sensitive
	}

	if input.SpoilerText != nil {
		cmd.SpoilerText = *input.SpoilerText
	}

	if input.InReplyToID != nil {
		cmd.InReplyToID = *input.InReplyToID
	}

	if input.Language != nil {
		cmd.Language = *input.Language
	}

	if input.MediaIds != nil {
		cmd.MediaIDs = input.MediaIds
	}

	if input.Poll != nil {
		cmd.Poll = map[string]any{
			"options":     input.Poll.Options,
			"expires_in":  input.Poll.ExpiresIn,
			"multiple":    input.Poll.Multiple != nil && *input.Poll.Multiple,
			"hide_totals": input.Poll.HideTotals != nil && *input.Poll.HideTotals,
		}
	}

	result, err := r.Registry.Scheduled().CreateScheduledStatus(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to create scheduled status",
			zap.String("user", username),
			zap.Time("scheduledAt", scheduledTime),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to schedule status"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertScheduledStatusToGraphQL(ctx, result.ScheduledStatus), nil
}

// UpdateScheduledStatus is the resolver for the updateScheduledStatus field.
func (r *mutationResolver) UpdateScheduledStatus(ctx context.Context, id string, input model.UpdateScheduledStatusInput) (*model.ScheduledStatus, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	scheduledTime := time.Time(input.ScheduledAt)
	if scheduledTime.Before(time.Now().Add(5 * time.Minute)) {
		return nil, ErrScheduledTimeMinimum
	}
	if scheduledTime.After(time.Now().AddDate(1, 0, 0)) {
		return nil, ErrScheduledTimeMaximum
	}

	result, err := r.Registry.Scheduled().UpdateScheduledStatus(ctx, &scheduled.UpdateScheduledStatusCommand{
		Username:    username,
		ID:          id,
		ScheduledAt: &scheduledTime,
	})
	if err != nil {
		r.Logger.Error("Failed to update scheduled status",
			zap.String("user", username),
			zap.String("id", id),
			zap.Time("newTime", scheduledTime),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update scheduled status"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertScheduledStatusToGraphQL(ctx, result.ScheduledStatus), nil
}

// CancelScheduledStatus is the resolver for the cancelScheduledStatus field.
func (r *mutationResolver) CancelScheduledStatus(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	err = r.Registry.Scheduled().DeleteScheduledStatus(ctx, &scheduled.DeleteScheduledStatusCommand{
		Username: username,
		ID:       id,
	})
	if err != nil {
		r.Logger.Error("Failed to cancel scheduled status",
			zap.String("user", username),
			zap.String("id", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to cancel scheduled status"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

// CreateQuoteNote implements MutationResolver
func (r *mutationResolver) CreateQuoteNote(ctx context.Context, input model.CreateQuoteNoteInput) (*model.CreateNotePayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Set default visibility if not provided
	// Convert GraphQL enum (PUBLIC, UNLISTED, etc.) to lowercase (public, unlisted, etc.)
	visibility := "public"
	if input.Visibility != nil {
		visibility = strings.ToLower(string(*input.Visibility))
	}

	// Create the quote note using the notes service
	// Note: QuoteURL would need to be parsed to extract the note ID for quoting
	cmd := &notes.CreateNoteCommand{
		AuthorID:   username,
		Content:    fmt.Sprintf("%s\n\nQuoting: %s", input.Content, input.QuoteURL),
		Visibility: visibility,
		Sensitive:  input.Sensitive != nil && *input.Sensitive,
		MediaIDs:   input.MediaIds,
	}

	if input.SpoilerText != nil {
		cmd.SpoilerText = *input.SpoilerText
	}

	result, err := r.Registry.Notes().CreateNote(ctx, cmd)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create quote note"), err)
	}

	// Convert result to GraphQL model
	if result.Note == nil {
		return nil, ErrNoteCreationReturnedNoNote
	}

	return &model.CreateNotePayload{
		Object: r.convertStatusToObject(ctx, result.Note),
	}, nil
}
