package graph

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

func mapStatusVisibility(status *models.Status) model.Visibility {
	if status == nil {
		return model.VisibilityPublic
	}

	switch status.Visibility {
	case VisibilityUnlisted:
		return model.VisibilityUnlisted
	case VisibilityPrivate, EventTypeFollowers:
		return model.VisibilityFollowers
	case TimelineTypeDirect:
		return model.VisibilityDirect
	default:
		return model.VisibilityPublic
	}
}

func cloneNoteAttachments(status *models.Status) []*activitypub.Attachment {
	if status == nil || status.Note == nil || status.Note.Get() == nil {
		return []*activitypub.Attachment{}
	}

	attachments := make([]*activitypub.Attachment, 0, len(status.Note.Get().Attachment))
	for _, att := range status.Note.Get().Attachment {
		attachment := att
		attachments = append(attachments, &attachment)
	}

	return attachments
}

func cloneNoteTags(status *models.Status) []*activitypub.Tag {
	if status == nil || status.Note == nil || status.Note.Get() == nil {
		return []*activitypub.Tag{}
	}

	tags := make([]*activitypub.Tag, 0, len(status.Note.Get().Tag))
	for _, tag := range status.Note.Get().Tag {
		tagCopy := tag
		tags = append(tags, &tagCopy)
	}

	return tags
}

func (r *Resolver) buildMentions(status *models.Status) []*model.Mention {
	if status == nil || status.Mentions == nil {
		return []*model.Mention{}
	}

	mentions := make([]*model.Mention, 0, len(status.Mentions))
	for _, mentionURL := range status.Mentions {
		username, domain := r.parseMentionURL(mentionURL)
		if username == "" {
			continue
		}
		mentions = append(mentions, &model.Mention{
			ID:       mentionURL,
			Username: username,
			URL:      mentionURL,
			Domain:   domain,
		})
	}

	return mentions
}

func determineQuoteable(status *models.Status) bool {
	if status == nil || status.Note == nil || status.Note.Get() == nil {
		return true
	}
	if status.Note.Get().Quoteable {
		return status.Note.Get().Quoteable
	}
	return true
}

func extractStatusSummary(status *models.Status) *string {
	if status == nil || status.Note == nil || status.Note.Get() == nil {
		return nil
	}
	if status.Note.Get().Summary == "" {
		return nil
	}
	summary := status.Note.Get().Summary
	return &summary
}

func (r *Resolver) viewerBoostState(ctx context.Context, status *models.Status, convertLogger *zap.Logger) bool {
	if status == nil || status.StatusID == "" {
		return false
	}

	viewerUsername := getUsernameFromContext(ctx)
	if viewerUsername == "" {
		return false
	}

	boosted, err := viewerBoostStateResolverFunc(ctx, r, viewerUsername, status.StatusID)
	if err != nil && convertLogger != nil {
		convertLogger.Warn("failed to resolve viewer boost state",
			zap.String("status_id", status.StatusID),
			zap.String("viewer", viewerUsername),
			zap.Error(err))
	}

	return err == nil && boosted
}

func (r *Resolver) resolveActorForStatus(ctx context.Context, status *models.Status, convertLogger *zap.Logger) *activitypub.Actor {
	if status == nil || status.AuthorUsername == "" || r.Registry == nil {
		return nil
	}

	accountStart := time.Now()
	result, err := r.Registry.Accounts().GetAccount(ctx, status.AuthorUsername)
	if convertLogger != nil {
		convertLogger.Info("convertStatusToObject account lookup",
			zap.String("status_id", status.StatusID),
			zap.String("author_username", status.AuthorUsername),
			zap.Duration("duration", time.Since(accountStart)),
			zap.Bool("found", err == nil && result != nil),
			zap.Error(err))
	}
	if err != nil || result == nil {
		return nil
	}
	return r.convertAccountToActor(result)
}

func (r *Resolver) resolveInReplyToObject(ctx context.Context, status *models.Status, convertLogger *zap.Logger) *model.Object {
	if status == nil || status.InReplyToID == "" {
		return nil
	}

	depth := r.getConversionDepth(ctx)
	if depth >= 3 {
		return nil
	}

	newCtx := r.setConversionDepth(ctx, depth+1)
	parentLookupStart := time.Now()
	parentStatus, err := r.Registry.Notes().GetNote(newCtx, status.InReplyToID)
	if convertLogger != nil {
		convertLogger.Info("convertStatusToObject parent lookup",
			zap.String("status_id", status.StatusID),
			zap.String("in_reply_to", status.InReplyToID),
			zap.Duration("duration", time.Since(parentLookupStart)),
			zap.Error(err))
	}
	if err != nil || parentStatus == nil {
		return nil
	}

	return r.convertStatusToObject(newCtx, parentStatus)
}
