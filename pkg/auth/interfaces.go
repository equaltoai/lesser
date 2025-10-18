package auth

import (
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// StorageProvider is the common interface for all auth services to access storage repositories
// This avoids importing pkg/storage/core which causes import cycles
type StorageProvider interface {
	Account() *repositories.AccountRepository
	Actor() *repositories.ActorRepository
	Activity() *repositories.ActivityRepository
	Notification() *repositories.NotificationRepository
	Recovery() *repositories.RecoveryRepository
	Audit() *repositories.AuditRepository
}
