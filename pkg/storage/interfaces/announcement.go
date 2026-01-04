// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// AnnouncementRepository defines the interface for announcement operations.
// This handles instance-wide announcements and user interactions with them.
type AnnouncementRepository interface {
	// CreateAnnouncement creates a new announcement
	CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error

	// GetAnnouncement retrieves a single announcement by ID
	GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error)

	// GetAnnouncements retrieves all announcements (for backward compatibility)
	GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error)

	// GetAnnouncementsPaginated retrieves announcements with pagination
	GetAnnouncementsPaginated(ctx context.Context, active bool, limit int, cursor string) ([]*storage.Announcement, string, error)

	// GetAnnouncementsByAdmin retrieves announcements created by a specific admin
	GetAnnouncementsByAdmin(ctx context.Context, adminUsername string, limit int, cursor string) ([]*storage.Announcement, string, error)

	// UpdateAnnouncement updates an existing announcement
	UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error

	// DeleteAnnouncement deletes an announcement
	DeleteAnnouncement(ctx context.Context, id string) error

	// DismissAnnouncement marks an announcement as dismissed by a user
	DismissAnnouncement(ctx context.Context, username, announcementID string) error

	// IsDismissed checks if a user has dismissed an announcement
	IsDismissed(ctx context.Context, username, announcementID string) (bool, error)

	// GetDismissedAnnouncements gets all announcement IDs dismissed by a user
	GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error)

	// AddAnnouncementReaction adds a user's reaction to an announcement
	AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error

	// RemoveAnnouncementReaction removes a user's reaction from an announcement
	RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error

	// GetAnnouncementReactions gets all reactions for an announcement
	GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error)
}
