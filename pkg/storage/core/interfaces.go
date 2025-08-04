package core

import (
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// RepositoryStorage is the new minimal storage interface that exposes only repository access
// This replaces the massive Storage interface with a clean, repository-focused approach
type RepositoryStorage interface {
	// Repository access methods - only expose the core repositories that are actually used
	Account() *repositories.AccountRepository
	Actor() *repositories.ActorRepository
	Object() *repositories.ObjectRepository
	Activity() *repositories.ActivityRepository
	Timeline() *repositories.TimelineRepository
	Notification() *repositories.NotificationRepository
	Like() *repositories.LikeRepository
	Moderation() *repositories.ModerationRepository
	List() *repositories.ListRepository
	Media() *repositories.MediaRepository
	Poll() *repositories.PollRepository
	Hashtag() *repositories.HashtagRepository
	ScheduledStatus() *repositories.ScheduledStatusRepository
	Announcement() *repositories.AnnouncementRepository
	DomainBlock() *repositories.DomainBlockRepository
	Relationship() *repositories.RelationshipRepository
	Instance() *repositories.InstanceRepository
	Federation() *repositories.FederationRepository
	Recovery() *repositories.RecoveryRepository
	Analytics() *repositories.TrendingRepository // Analytics/Trending repository
	Social() *repositories.SocialRepository
	User() *repositories.UserRepository
	Status() *repositories.StatusRepository
	Cost() *repositories.CostTrackingRepository
	Trust() *repositories.TrustRepository
	Search() *repositories.SearchRepository
	
	// Utility methods
	GetDB() dynamormCore.DB
	GetTableName() string
	GetLogger() *zap.Logger
}