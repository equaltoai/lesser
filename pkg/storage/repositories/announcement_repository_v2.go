package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// AnnouncementRepositoryV2 implements announcement operations using BaseRepository
// This demonstrates how to use BaseRepository to reduce code duplication
type AnnouncementRepositoryV2 struct {
	*BaseRepository[*models.Announcement]
	logger *zap.Logger
}

// NewAnnouncementRepositoryV2 creates a new announcement repository using BaseRepository
func NewAnnouncementRepositoryV2(db core.DB, tableName string, logger *zap.Logger) *AnnouncementRepositoryV2 {
	return &AnnouncementRepositoryV2{
		BaseRepository: NewBaseRepository[*models.Announcement](db, tableName, logger),
		logger:         logger,
	}
}

// CreateAnnouncement creates a new announcement
func (r *AnnouncementRepositoryV2) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	// Convert to model
	model := &models.Announcement{
		PK:          fmt.Sprintf("ANNOUNCEMENT#%s", announcement.ID),
		SK:          "METADATA",
		ID:          announcement.ID,
		Content:     announcement.Content,
		Text:        announcement.Text,
		Tags:        announcement.Tags,
		StartsAt:    announcement.StartsAt,
		EndsAt:      announcement.EndsAt,
		AllDay:      announcement.AllDay,
		PublishedAt: announcement.PublishedAt,
		UpdatedAt:   announcement.UpdatedAt,
		CreatedBy:   announcement.CreatedBy,
		CreatedAt:   time.Now(),
	}
	
	// Convert emojis and reactions
	if len(announcement.Emojis) > 0 {
		model.Emojis = make([]models.CustomEmoji, len(announcement.Emojis))
		for i, e := range announcement.Emojis {
			model.Emojis[i] = models.CustomEmoji{
				Shortcode:       e.Shortcode,
				URL:             e.URL,
				StaticURL:       e.StaticURL,
				VisibleInPicker: e.VisibleInPicker,
				Category:        e.Category,
			}
		}
	}
	
	if len(announcement.Reactions) > 0 {
		model.Reactions = make([]models.Reaction, len(announcement.Reactions))
		for i, r := range announcement.Reactions {
			model.Reactions[i] = models.Reaction{
				Name:      r.Name,
				Count:     r.Count,
				Me:        r.Me,
				URL:       r.URL,
				StaticURL: r.StaticURL,
			}
		}
	}

	// Use BaseRepository Create - saves ~20 lines of boilerplate
	return r.Create(ctx, model)
}

// GetAnnouncement retrieves an announcement by ID
func (r *AnnouncementRepositoryV2) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	pk := fmt.Sprintf("ANNOUNCEMENT#%s", id)
	sk := "METADATA"
	
	model := &models.Announcement{}
	
	// Use BaseRepository Get - saves ~15 lines of boilerplate
	err := r.Get(ctx, pk, sk, model)
	if err != nil {
		return nil, err
	}
	
	// Convert model back to storage type
	return r.modelToAnnouncement(model), nil
}

// UpdateAnnouncement updates an announcement
func (r *AnnouncementRepositoryV2) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	// Get existing announcement model to preserve creation time
	existingModel := &models.Announcement{}
	err := r.Get(ctx, fmt.Sprintf("ANNOUNCEMENT#%s", announcement.ID), "METADATA", existingModel)
	if err != nil {
		return err
	}
	
	// Convert to model with updates
	model := &models.Announcement{
		PK:          fmt.Sprintf("ANNOUNCEMENT#%s", announcement.ID),
		SK:          "METADATA",
		ID:          announcement.ID,
		Content:     announcement.Content,
		Text:        announcement.Text,
		Tags:        announcement.Tags,
		StartsAt:    announcement.StartsAt,
		EndsAt:      announcement.EndsAt,
		AllDay:      announcement.AllDay,
		PublishedAt: announcement.PublishedAt,
		UpdatedAt:   time.Now(),
		CreatedBy:   announcement.CreatedBy,
		CreatedAt:   existingModel.CreatedAt, // Preserve original creation time
	}
	
	// Convert emojis and reactions
	if len(announcement.Emojis) > 0 {
		model.Emojis = make([]models.CustomEmoji, len(announcement.Emojis))
		for i, e := range announcement.Emojis {
			model.Emojis[i] = models.CustomEmoji{
				Shortcode:       e.Shortcode,
				URL:             e.URL,
				StaticURL:       e.StaticURL,
				VisibleInPicker: e.VisibleInPicker,
				Category:        e.Category,
			}
		}
	}
	
	if len(announcement.Reactions) > 0 {
		model.Reactions = make([]models.Reaction, len(announcement.Reactions))
		for i, r := range announcement.Reactions {
			model.Reactions[i] = models.Reaction{
				Name:      r.Name,
				Count:     r.Count,
				Me:        r.Me,
				URL:       r.URL,
				StaticURL: r.StaticURL,
			}
		}
	}
	
	// Use BaseRepository Update - saves ~15 lines of boilerplate
	return r.Update(ctx, model)
}

// DeleteAnnouncement removes an announcement
func (r *AnnouncementRepositoryV2) DeleteAnnouncement(ctx context.Context, id string) error {
	pk := fmt.Sprintf("ANNOUNCEMENT#%s", id)
	sk := "METADATA"
	
	// Use BaseRepository Delete - saves ~15 lines of boilerplate
	return r.Delete(ctx, pk, sk)
}

// GetActiveAnnouncements retrieves all active announcements
func (r *AnnouncementRepositoryV2) GetActiveAnnouncements(ctx context.Context) ([]*storage.Announcement, error) {
	now := time.Now()
	
	// Use BaseRepository Query - saves ~20 lines of boilerplate
	models, err := r.Query(ctx, "ANNOUNCEMENT#", 100)
	if err != nil {
		return nil, err
	}
	
	// Filter active announcements
	active := make([]*storage.Announcement, 0)
	for _, model := range models {
		ann := r.modelToAnnouncement(model)
		
		// Check if announcement is active based on time range
		// An announcement is active if current time is between start and end times
		if (ann.StartsAt == nil || now.After(*ann.StartsAt)) &&
		   (ann.EndsAt == nil || now.Before(*ann.EndsAt)) {
			active = append(active, ann)
		}
	}
	
	return active, nil
}

// GetAnnouncementCount returns the total number of announcements
func (r *AnnouncementRepositoryV2) GetAnnouncementCount(ctx context.Context) (int, error) {
	// Use BaseRepository Count - saves ~15 lines of boilerplate
	return r.Count(ctx, "ANNOUNCEMENT#")
}

// Helper to convert model to storage type
func (r *AnnouncementRepositoryV2) modelToAnnouncement(model *models.Announcement) *storage.Announcement {
	ann := &storage.Announcement{
		ID:          model.ID,
		Content:     model.Content,
		Text:        model.Text,
		Tags:        model.Tags,
		StartsAt:    model.StartsAt,
		EndsAt:      model.EndsAt,
		AllDay:      model.AllDay,
		PublishedAt: model.PublishedAt,
		UpdatedAt:   model.UpdatedAt,
		CreatedBy:   model.CreatedBy,
	}
	
	// Convert emojis
	if len(model.Emojis) > 0 {
		ann.Emojis = make([]storage.CustomEmoji, len(model.Emojis))
		for i, e := range model.Emojis {
			ann.Emojis[i] = storage.CustomEmoji{
				Shortcode:       e.Shortcode,
				URL:             e.URL,
				StaticURL:       e.StaticURL,
				VisibleInPicker: e.VisibleInPicker,
				Category:        e.Category,
			}
		}
	}
	
	// Convert reactions
	if len(model.Reactions) > 0 {
		ann.Reactions = make([]storage.Reaction, len(model.Reactions))
		for i, r := range model.Reactions {
			ann.Reactions[i] = storage.Reaction{
				Name:      r.Name,
				Count:     r.Count,
				Me:        r.Me,
				URL:       r.URL,
				StaticURL: r.StaticURL,
			}
		}
	}
	
	return ann
}

// Code Reduction Summary:
// - CreateAnnouncement: ~20 lines saved (error handling, logging, key updates)
// - GetAnnouncement: ~15 lines saved (query construction, error handling)
// - UpdateAnnouncement: ~15 lines saved (update logic, error handling)
// - DeleteAnnouncement: ~15 lines saved (delete logic, error handling)
// - GetActiveAnnouncements: ~20 lines saved (query construction)
// - GetAnnouncementCount: ~15 lines saved (count query)
// Total: ~100 lines of boilerplate code eliminated!