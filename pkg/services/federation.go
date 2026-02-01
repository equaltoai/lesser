package services

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// federationService implements FederationService
type federationService struct {
	deps    *ServiceDependencies
	storage StorageAdapter
	logger  *zap.Logger
}

// NewFederationService creates a new federation service
func NewFederationService(deps *ServiceDependencies) FederationService {
	return &federationService{
		deps:    deps,
		storage: CreateStorageAdapter(deps.Repos),
		logger:  deps.Logger.(*zap.Logger),
	}
}

// DeliverToFollowers delivers an activity to all followers of the actor
func (f *federationService) DeliverToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Use storage directly if it implements FederationStorage
	var federationStorage federation.FederationStorage

	// Try to get DB from storage for DynamORM federation storage
	if db := f.storage.GetDB(); db != nil {
		// Check if it's a core.DB type
		if coreDB, ok := db.(core.DB); ok {
			federationStorage = federation.NewDynamORMFederationStorage(coreDB, f.storage.GetTableName(), f.deps.Logger.(*zap.Logger))
		} else {
			// Fall back to a basic implementation or return error
			return ErrUnsupportedDatabaseType
		}
	} else {
		return ErrNoDatabaseAvailable
	}

	deliveryService := federation.NewDeliveryService(federationStorage, f.deps.Config.Config)
	return deliveryService.DeliverToFollowers(ctx, activity, actor)
}

// DeliverToRecipients delivers an activity to specific recipients
func (f *federationService) DeliverToRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Use storage directly if it implements FederationStorage
	var federationStorage federation.FederationStorage

	// Try to get DB from storage for DynamORM federation storage
	if db := f.storage.GetDB(); db != nil {
		// Check if it's a core.DB type
		if coreDB, ok := db.(core.DB); ok {
			federationStorage = federation.NewDynamORMFederationStorage(coreDB, f.storage.GetTableName(), f.deps.Logger.(*zap.Logger))
		} else {
			// Fall back to a basic implementation or return error
			return ErrUnsupportedDatabaseType
		}
	} else {
		return ErrNoDatabaseAvailable
	}

	deliveryService := federation.NewDeliveryService(federationStorage, f.deps.Config.Config)

	return deliveryService.DeliverToRecipients(ctx, activity, actor)
}

// DetermineRecipients determines who should receive an activity based on its type and visibility
func (f *federationService) DetermineRecipients(ctx context.Context, activity *activitypub.Activity, visibility string) ([]string, error) {
	recipients := make(map[string]bool)

	// Extract actor from activity
	actorID := activity.Actor
	if err := common.ValidateRequiredParam("actor_id", actorID); err != nil {
		return nil, ErrActivityMissingActor
	}

	// Get actor username for follower lookup
	actor, err := f.getActorByID(ctx, actorID)
	if err != nil {
		f.logger.Warn("failed to get actor for recipient determination", zap.Error(err))
		return []string{}, nil
	}

	// Add recipients based on visibility
	switch visibility {
	case "public", "unlisted":
		// Add all followers
		followers, err := f.getFollowers(ctx, actor.PreferredUsername)
		if err != nil {
			f.logger.Warn("failed to get followers for delivery", zap.Error(err))
		} else {
			for _, follower := range followers {
				recipients[follower] = true
			}
		}

		// Add mentioned users
		mentions := f.extractMentions(activity)
		for _, mention := range mentions {
			recipients[mention] = true
		}

	case "private":
		// Only followers
		followers, err := f.getFollowers(ctx, actor.PreferredUsername)
		if err != nil {
			f.logger.Warn("failed to get followers for private delivery", zap.Error(err))
		} else {
			for _, follower := range followers {
				recipients[follower] = true
			}
		}

	case "direct":
		// Only mentioned users
		mentions := f.extractMentions(activity)
		for _, mention := range mentions {
			recipients[mention] = true
		}
	}

	// Add users from To/CC fields
	if activity.To != nil {
		for _, recipient := range activity.To {
			if recipient != activitypub.PublicAddress {
				recipients[recipient] = true
			}
		}
	}

	if activity.CC != nil {
		for _, recipient := range activity.CC {
			if recipient != activitypub.PublicAddress {
				recipients[recipient] = true
			}
		}
	}

	// Convert to slice
	recipientList := make([]string, 0, len(recipients))
	for recipient := range recipients {
		recipientList = append(recipientList, recipient)
	}

	return recipientList, nil
}

// Helper methods

func (f *federationService) getActorByID(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	// Extract username from actor ID
	// Format: https://domain/users/username or https://domain/@username
	username := ""
	if strings.Contains(actorID, "/users/") {
		parts := strings.Split(actorID, "/users/")
		if len(parts) == 2 {
			username = parts[1]
		}
	} else if strings.Contains(actorID, "/@") {
		parts := strings.Split(actorID, "/@")
		if len(parts) == 2 {
			username = parts[1]
		}
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		f.logger.Error("invalid actor ID format", zap.String("actor_id", actorID))
		return nil, ErrInvalidActorIDFormat
	}

	return f.storage.GetActor(ctx, username)
}

func (f *federationService) getFollowers(ctx context.Context, username string) ([]string, error) {
	followers, _, err := f.storage.GetFollowers(ctx, username, 1000, "")
	return followers, err
}

func (f *federationService) extractMentions(activity *activitypub.Activity) []string {
	var mentions []string

	// Extract mentions from the activity object if it's a Note
	if note, ok := activity.Object.(*activitypub.Note); ok {
		for _, tag := range note.Tag {
			if tag.Type == "Mention" && tag.Href != "" {
				mentions = append(mentions, tag.Href)
			}
		}
	} else if article, ok := activity.Object.(*activitypub.Article); ok {
		// Extract mentions from the activity object if it's an Article
		for _, tag := range article.Tag {
			if tag.Type == "Mention" && tag.Href != "" {
				mentions = append(mentions, tag.Href)
			}
		}
	}

	return mentions
}

// GetInstanceRelationships returns federation relationships for a domain
func (f *federationService) GetInstanceRelationships(ctx context.Context, domain string) (*model.InstanceRelations, error) {
	// Query storage for federation relationships
	relationships, err := f.storage.GetInstanceRelationships(ctx, domain)
	if err != nil {
		f.logger.Error("failed to get instance relationships",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, err
	}
	return relationships, nil
}
