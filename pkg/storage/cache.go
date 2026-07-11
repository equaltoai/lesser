package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

// CacheService provides simple caching using DynamORM
type CacheService struct {
	db     core.DB
	logger *zap.Logger
}

// NewCacheService creates a new cache service
func NewCacheService(db core.DB, logger *zap.Logger) *CacheService {
	return &CacheService{
		db:     db,
		logger: logger,
	}
}

// CacheUser stores a user in cache
func (cs *CacheService) CacheUser(ctx context.Context, userID string, user *models.User) error {
	cacheModel := &models.User{
		PK:        fmt.Sprintf("CACHE#USER#%s", userID),
		SK:        "DATA",
		Username:  user.Username,
		CreatedAt: time.Now(),
	}

	return cs.db.WithContext(ctx).Model(cacheModel).Create()
}

// GetCachedUser retrieves a user from cache
func (cs *CacheService) GetCachedUser(ctx context.Context, userID string) (*models.User, error) {
	var cacheModel models.User
	err := cs.db.WithContext(ctx).Model(&models.User{}).
		Where("PK", "=", fmt.Sprintf("CACHE#USER#%s", userID)).
		Where("SK", "=", "DATA").
		First(&cacheModel)

	if err != nil {
		return nil, err
	}

	return &cacheModel, nil
}

// InvalidateUser removes a user from cache
func (cs *CacheService) InvalidateUser(ctx context.Context, userID string) error {
	cacheModel := &models.User{
		PK: fmt.Sprintf("CACHE#USER#%s", userID),
		SK: "DATA",
	}

	return cs.db.WithContext(ctx).Model(cacheModel).Delete()
}
