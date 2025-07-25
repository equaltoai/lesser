package mastodon

import (
	"context"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
)

// Converter handles all ActivityPub to Mastodon API conversions
type Converter interface {
	// Actor conversions
	ActorToAccount(actor *activitypub.Actor) models.Account
	ActorToAccountWithCounts(actor *activitypub.Actor, followers, following, statuses int) models.Account
	ActorToAccountWithMetadata(actor *activitypub.Actor, metadata *storage.ActorMetadata, followers, following, statuses int) models.Account

	// Object conversions
	ObjectToStatus(obj any, actor *activitypub.Actor) models.Status
	ObjectToStatusWithContext(ctx context.Context, obj any, actor *activitypub.Actor, likeCount, reblogCount int, favorited, reblogged, bookmarked bool) models.Status

	// Conversation conversions
	ConversationToAPI(conv *storage.Conversation, participants []*activitypub.Actor, lastStatus any, unread bool) models.Conversation

	// Filter conversions
	ConvertFilterToMastodon(filter *storage.Filter, keywords []*storage.FilterKeyword, statuses []*storage.FilterStatus) *Filter
	ConvertFilterKeywordToV1(keyword *storage.FilterKeyword, filter *storage.Filter) *V1Filter

	// Mute conversions
	ConvertMuteToRelationship(relationship *models.Relationship, mute *storage.Mute)

	// Notes conversions
	NotesToStatus(note any) models.Status

	// Utility methods
	ExtractUsernameFromActorID(actorID string) string
	ExtractIDFromURL(url string) string
}

// ActorService provides higher-level actor operations
type ActorService interface {
	GetAccountByUsername(ctx context.Context, username string) (*models.Account, error)
	GetAccountWithStats(ctx context.Context, username string) (*models.Account, error)
	GetAccountsByIDs(ctx context.Context, actorIDs []string) ([]models.Account, error)
}
