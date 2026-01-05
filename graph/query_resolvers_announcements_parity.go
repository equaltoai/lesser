package graph

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// Announcements returns active announcements for the instance.
func (r *queryResolver) Announcements(ctx context.Context) ([]*model.Announcement, error) {
	if r.Storage == nil || r.Storage.Announcement() == nil {
		return nil, ErrStorageUnavailable
	}

	username := r.optionalAuth(ctx)

	announcements, err := r.Storage.Announcement().GetAnnouncements(ctx, true)
	if err != nil {
		r.Logger.Error("failed to get announcements", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get announcements"), err)
	}

	dismissedIDs := make(map[string]bool)
	if username != "" {
		dismissed, err := r.Storage.Announcement().GetDismissedAnnouncements(ctx, username)
		if err != nil {
			r.Logger.Warn("failed to get dismissed announcements",
				zap.String("username", username),
				zap.Error(err))
		} else {
			for _, id := range dismissed {
				dismissedIDs[id] = true
			}
		}
	}

	result := make([]*model.Announcement, 0, len(announcements))
	for _, announcement := range announcements {
		if announcement == nil {
			continue
		}
		if dismissedIDs[announcement.ID] {
			continue
		}

		startsAt := (*model.Time)(nil)
		if announcement.StartsAt != nil {
			t := model.Time(*announcement.StartsAt)
			startsAt = &t
		}

		endsAt := (*model.Time)(nil)
		if announcement.EndsAt != nil {
			t := model.Time(*announcement.EndsAt)
			endsAt = &t
		}

		result = append(result, &model.Announcement{
			ID:          announcement.ID,
			Content:     announcement.Content,
			Text:        announcement.Text,
			PublishedAt: model.Time(announcement.PublishedAt),
			UpdatedAt:   model.Time(announcement.UpdatedAt),
			AllDay:      announcement.AllDay,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
			Read:        false,
			Reactions:   r.buildAnnouncementReactions(ctx, announcement.ID, username, announcement.Reactions),
		})
	}

	return result, nil
}

func (r *queryResolver) buildAnnouncementReactions(ctx context.Context, announcementID string, username string, available []storage.Reaction) []*model.AnnouncementReaction {
	if r.Storage == nil || r.Storage.Announcement() == nil {
		return []*model.AnnouncementReaction{}
	}

	reactions, err := r.Storage.Announcement().GetAnnouncementReactions(ctx, announcementID)
	if err != nil {
		r.Logger.Warn("failed to get announcement reactions",
			zap.String("announcement_id", announcementID),
			zap.Error(err))
		reactions = make(map[string][]string)
	}

	actual := make(map[string]*model.AnnouncementReaction)
	for emojiName, users := range reactions {
		me := false
		if username != "" {
			for _, user := range users {
				if user == username {
					me = true
					break
				}
			}
		}

		reaction := &model.AnnouncementReaction{
			Name:  emojiName,
			Count: len(users),
			Me:    me,
		}

		r.populateAnnouncementReactionEmoji(ctx, reaction)
		actual[emojiName] = reaction
	}

	// If there are no pre-defined reactions, return actual reactions (stable order).
	if err := common.ValidateSliceNotEmpty("available_reactions", available); err != nil {
		names := make([]string, 0, len(actual))
		for name := range actual {
			names = append(names, name)
		}
		sort.Strings(names)

		out := make([]*model.AnnouncementReaction, 0, len(names))
		for _, name := range names {
			out = append(out, actual[name])
		}
		return out
	}

	out := make([]*model.AnnouncementReaction, 0, len(available))
	for _, avail := range available {
		name := strings.TrimSpace(avail.Name)
		if name == "" {
			continue
		}

		reaction := actual[name]
		if reaction == nil {
			reaction = &model.AnnouncementReaction{
				Name:      name,
				Count:     0,
				Me:        false,
				URL:       optionalString(avail.URL),
				StaticURL: optionalString(avail.StaticURL),
			}
			r.populateAnnouncementReactionEmoji(ctx, reaction)
		}

		out = append(out, reaction)
	}

	return out
}

func (r *queryResolver) populateAnnouncementReactionEmoji(ctx context.Context, reaction *model.AnnouncementReaction) {
	if reaction == nil || r.Storage == nil || r.Storage.Emoji() == nil {
		return
	}

	name := strings.TrimSpace(reaction.Name)
	if !strings.HasPrefix(name, ":") || !strings.HasSuffix(name, ":") {
		return
	}

	shortcode := strings.Trim(strings.TrimPrefix(strings.TrimSuffix(name, ":"), ":"), " ")
	if shortcode == "" {
		return
	}

	emoji, err := r.Storage.Emoji().GetCustomEmoji(ctx, shortcode)
	if err != nil || emoji == nil || emoji.Disabled {
		return
	}

	reaction.URL = optionalString(emoji.URL)
	reaction.StaticURL = optionalString(emoji.StaticURL)
}
