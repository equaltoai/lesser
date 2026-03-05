package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// InstanceRepository implements instance operations using enhanced DynamORM patterns
type InstanceRepository struct {
	*EnhancedBaseRepository[*models.InstanceConfig]
	historyRepo                  *BaseRepository[*models.InstanceHistory]
	metricsRepo                  *BaseRepository[*models.InstanceMetrics]
	activityRepo                 *BaseRepository[*models.WeeklyActivity]
	stateRepo                    *BaseRepository[*models.InstanceState]
	agentRepo                    *BaseRepository[*models.AgentInstanceConfig]
	trustRepo                    *BaseRepository[*models.InstanceTrustConfig]
	translationRepo              *BaseRepository[*models.InstanceTranslationConfig]
	tipsRepo                     *BaseRepository[*models.InstanceTipsConfig]
	aiConfigRepo                 *BaseRepository[*models.AIInstanceConfig]
	wellKnownLesserSoulAgentRepo *BaseRepository[*models.InstanceWellKnownLesserSoulAgent]
	logger                       *zap.Logger

	stateCache                    instanceStateCache
	agentCache                    agentInstanceConfigCache
	trustCache                    instanceTrustConfigCache
	translationCache              instanceTranslationConfigCache
	tipsCache                     instanceTipsConfigCache
	aiConfigCache                 aiInstanceConfigCache
	wellKnownLesserSoulAgentCache wellKnownLesserSoulAgentCache
}

type instanceStateCache struct {
	mu        sync.RWMutex
	state     *models.InstanceState
	expiresAt time.Time
}

const instanceStateCacheTTL = 5 * time.Second

type agentInstanceConfigCache struct {
	mu        sync.RWMutex
	cfg       *models.AgentInstanceConfig
	expiresAt time.Time
}

const agentConfigCacheTTL = 5 * time.Second

type instanceTrustConfigCache struct {
	mu        sync.RWMutex
	cfg       *models.InstanceTrustConfig
	expiresAt time.Time
}

type instanceTranslationConfigCache struct {
	mu        sync.RWMutex
	cfg       *models.InstanceTranslationConfig
	expiresAt time.Time
}

type instanceTipsConfigCache struct {
	mu        sync.RWMutex
	cfg       *models.InstanceTipsConfig
	expiresAt time.Time
}

type aiInstanceConfigCache struct {
	mu        sync.RWMutex
	cfg       *models.AIInstanceConfig
	expiresAt time.Time
}

const instanceFeatureConfigCacheTTL = 5 * time.Second

type wellKnownLesserSoulAgentCache struct {
	mu        sync.RWMutex
	cfg       *models.InstanceWellKnownLesserSoulAgent
	expiresAt time.Time
}

const wellKnownLesserSoulAgentCacheTTL = 5 * time.Second

// NewInstanceRepository creates a new instance repository with enhanced functionality
func NewInstanceRepository(db core.DB, tableName string, logger *zap.Logger) *InstanceRepository {
	// Create enhanced repository optimized for instance operations
	enhancedRepo := NewEnhancedBaseRepository[*models.InstanceConfig](db, tableName, logger, nil, "InstanceRepository", "instance_config")

	// Set up enhanced services for instance operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Admin-only instance config
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Instance config cached heavily
	enhancedRepo.SetEventService(NewDefaultEventService())           // Important for instance change events

	return &InstanceRepository{
		EnhancedBaseRepository:       enhancedRepo,
		historyRepo:                  NewBaseRepository[*models.InstanceHistory](db, tableName, logger),
		metricsRepo:                  NewBaseRepository[*models.InstanceMetrics](db, tableName, logger),
		activityRepo:                 NewBaseRepository[*models.WeeklyActivity](db, tableName, logger),
		stateRepo:                    NewBaseRepository[*models.InstanceState](db, tableName, logger),
		agentRepo:                    NewBaseRepository[*models.AgentInstanceConfig](db, tableName, logger),
		trustRepo:                    NewBaseRepository[*models.InstanceTrustConfig](db, tableName, logger),
		translationRepo:              NewBaseRepository[*models.InstanceTranslationConfig](db, tableName, logger),
		tipsRepo:                     NewBaseRepository[*models.InstanceTipsConfig](db, tableName, logger),
		aiConfigRepo:                 NewBaseRepository[*models.AIInstanceConfig](db, tableName, logger),
		wellKnownLesserSoulAgentRepo: NewBaseRepository[*models.InstanceWellKnownLesserSoulAgent](db, tableName, logger),
		logger:                       logger,
	}
}

// NewInstanceRepositoryWithCostTracking creates a new instance repository with cost tracking
func NewInstanceRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *InstanceRepository {
	// Create enhanced repository with cost tracking
	enhancedRepo := NewEnhancedBaseRepository[*models.InstanceConfig](db, tableName, logger, costService, "InstanceRepository", "instance_config")

	// Set up enhanced services for instance operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Admin-only instance config
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Instance config cached heavily
	enhancedRepo.SetEventService(NewDefaultEventService())           // Important for instance change events

	return &InstanceRepository{
		EnhancedBaseRepository:       enhancedRepo,
		historyRepo:                  NewBaseRepositoryWithCostTracking[*models.InstanceHistory](db, tableName, logger, costService, "instance_history"),
		metricsRepo:                  NewBaseRepositoryWithCostTracking[*models.InstanceMetrics](db, tableName, logger, costService, "instance_metrics"),
		activityRepo:                 NewBaseRepositoryWithCostTracking[*models.WeeklyActivity](db, tableName, logger, costService, "instance_activity"),
		stateRepo:                    NewBaseRepositoryWithCostTracking[*models.InstanceState](db, tableName, logger, costService, "instance_state"),
		agentRepo:                    NewBaseRepositoryWithCostTracking[*models.AgentInstanceConfig](db, tableName, logger, costService, "agent_instance_config"),
		trustRepo:                    NewBaseRepositoryWithCostTracking[*models.InstanceTrustConfig](db, tableName, logger, costService, "instance_trust_config"),
		translationRepo:              NewBaseRepositoryWithCostTracking[*models.InstanceTranslationConfig](db, tableName, logger, costService, "instance_translation_config"),
		tipsRepo:                     NewBaseRepositoryWithCostTracking[*models.InstanceTipsConfig](db, tableName, logger, costService, "instance_tips_config"),
		aiConfigRepo:                 NewBaseRepositoryWithCostTracking[*models.AIInstanceConfig](db, tableName, logger, costService, "instance_ai_config"),
		wellKnownLesserSoulAgentRepo: NewBaseRepositoryWithCostTracking[*models.InstanceWellKnownLesserSoulAgent](db, tableName, logger, costService, "instance_well_known_lesser_soul_agent"),
		logger:                       logger,
	}
}

func (r *InstanceRepository) getCachedState() (*models.InstanceState, bool) {
	r.stateCache.mu.RLock()
	state := r.stateCache.state
	expiresAt := r.stateCache.expiresAt
	r.stateCache.mu.RUnlock()

	if state == nil || time.Now().After(expiresAt) {
		return nil, false
	}
	return state, true
}

func (r *InstanceRepository) setCachedState(state *models.InstanceState) {
	r.stateCache.mu.Lock()
	r.stateCache.state = state
	r.stateCache.expiresAt = time.Now().Add(instanceStateCacheTTL)
	r.stateCache.mu.Unlock()
}

func (r *InstanceRepository) invalidateStateCache() {
	r.stateCache.mu.Lock()
	r.stateCache.state = nil
	r.stateCache.expiresAt = time.Time{}
	r.stateCache.mu.Unlock()
}

func (r *InstanceRepository) getCachedAgentConfig() (*models.AgentInstanceConfig, bool) {
	r.agentCache.mu.RLock()
	cfg := r.agentCache.cfg
	expiresAt := r.agentCache.expiresAt
	r.agentCache.mu.RUnlock()

	if cfg == nil || time.Now().After(expiresAt) {
		return nil, false
	}
	return cfg, true
}

func (r *InstanceRepository) setCachedAgentConfig(cfg *models.AgentInstanceConfig) {
	r.agentCache.mu.Lock()
	r.agentCache.cfg = cfg
	r.agentCache.expiresAt = time.Now().Add(agentConfigCacheTTL)
	r.agentCache.mu.Unlock()
}

//nolint:unused // Reserved for future cache invalidation hooks.
func (r *InstanceRepository) invalidateAgentConfigCache() {
	r.agentCache.mu.Lock()
	r.agentCache.cfg = nil
	r.agentCache.expiresAt = time.Time{}
	r.agentCache.mu.Unlock()
}

// GetInstanceState returns the current instance activation state.
// If no state exists yet, it defaults to a locked state without persisting.
func (r *InstanceRepository) GetInstanceState(ctx context.Context) (*models.InstanceState, error) {
	if cached, ok := r.getCachedState(); ok {
		return cached, nil
	}

	state := &models.InstanceState{}
	err := r.stateRepo.Get(ctx, storage.InstanceConfigKey, "STATE", state)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			defaultState := models.NewDefaultInstanceState()
			r.setCachedState(defaultState)
			return defaultState, nil
		}
		return nil, err
	}

	r.setCachedState(state)
	return state, nil
}

// GetAgentInstanceConfig returns the current instance agent policy.
// If no record exists yet, it returns conservative defaults without persisting.
func (r *InstanceRepository) GetAgentInstanceConfig(ctx context.Context) (*models.AgentInstanceConfig, error) {
	if cached, ok := r.getCachedAgentConfig(); ok {
		return cached, nil
	}

	cfg := &models.AgentInstanceConfig{}
	err := r.agentRepo.Get(ctx, storage.InstanceConfigKey, "AGENT_CONFIG", cfg)
	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			defaultCfg := models.NewAgentInstanceConfig()
			r.setCachedAgentConfig(defaultCfg)
			return defaultCfg, nil
		}
		return nil, err
	}

	r.setCachedAgentConfig(cfg)
	return cfg, nil
}

// EnsureAgentInstanceConfig ensures the instance agent policy record exists and returns it.
func (r *InstanceRepository) EnsureAgentInstanceConfig(ctx context.Context) (*models.AgentInstanceConfig, error) {
	cfg := &models.AgentInstanceConfig{}
	err := r.agentRepo.Get(ctx, storage.InstanceConfigKey, "AGENT_CONFIG", cfg)
	if err == nil {
		r.setCachedAgentConfig(cfg)
		return cfg, nil
	}

	if !appErrors.HasCode(err, appErrors.CodeNotFound) {
		return nil, err
	}

	cfg = models.NewAgentInstanceConfig()
	if createErr := r.agentRepo.Create(ctx, cfg); createErr != nil {
		// If another writer created it concurrently, read it back.
		if appErrors.HasCode(createErr, appErrors.CodeAlreadyExists) || appErrors.HasCode(createErr, appErrors.CodeConflict) {
			cfg = &models.AgentInstanceConfig{}
			if err := r.agentRepo.Get(ctx, storage.InstanceConfigKey, "AGENT_CONFIG", cfg); err != nil {
				return nil, err
			}
			r.setCachedAgentConfig(cfg)
			return cfg, nil
		}
		return nil, createErr
	}

	r.setCachedAgentConfig(cfg)
	return cfg, nil
}

// SetAgentInstanceConfig updates the instance agent policy.
func (r *InstanceRepository) SetAgentInstanceConfig(ctx context.Context, cfg *models.AgentInstanceConfig) error {
	if cfg == nil {
		return fmt.Errorf("agent config is nil")
	}

	cfg.UpdatedAt = time.Now()
	if err := cfg.UpdateKeys(); err != nil {
		return err
	}

	if err := r.agentRepo.Update(ctx, cfg); err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			if err := r.agentRepo.Create(ctx, cfg); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	r.setCachedAgentConfig(cfg)
	return nil
}

// EnsureInstanceState ensures the instance state record exists and returns it.
func (r *InstanceRepository) EnsureInstanceState(ctx context.Context) (*models.InstanceState, error) {
	state := &models.InstanceState{}
	err := r.stateRepo.Get(ctx, storage.InstanceConfigKey, "STATE", state)
	if err == nil {
		r.setCachedState(state)
		return state, nil
	}

	if !appErrors.HasCode(err, appErrors.CodeNotFound) {
		return nil, err
	}

	state = models.NewDefaultInstanceState()
	if createErr := r.stateRepo.Create(ctx, state); createErr != nil {
		// If another writer created it concurrently, read it back.
		if appErrors.HasCode(createErr, appErrors.CodeAlreadyExists) {
			r.invalidateStateCache()
			return r.EnsureInstanceState(ctx)
		}
		return nil, createErr
	}

	r.setCachedState(state)
	return state, nil
}

// SetInstanceLocked updates the instance lock state.
func (r *InstanceRepository) SetInstanceLocked(ctx context.Context, locked bool) error {
	state, err := r.EnsureInstanceState(ctx)
	if err != nil {
		return err
	}

	state.Locked = locked
	if !locked && state.ActivatedAt == nil {
		now := time.Now()
		state.ActivatedAt = &now
	}
	state.UpdatedAt = time.Now()

	if err := r.stateRepo.Update(ctx, state); err != nil {
		return err
	}
	r.setCachedState(state)
	return nil
}

// SetBootstrapWalletAddress sets the bootstrap wallet address used for setup authentication.
func (r *InstanceRepository) SetBootstrapWalletAddress(ctx context.Context, address string) error {
	state, err := r.EnsureInstanceState(ctx)
	if err != nil {
		return err
	}

	state.BootstrapWalletAddress = strings.ToLower(strings.TrimSpace(address))
	state.UpdatedAt = time.Now()

	if err := r.stateRepo.Update(ctx, state); err != nil {
		return err
	}
	r.setCachedState(state)
	return nil
}

// SetPrimaryAdminUsername records the primary admin username created during setup.
func (r *InstanceRepository) SetPrimaryAdminUsername(ctx context.Context, username string) error {
	state, err := r.EnsureInstanceState(ctx)
	if err != nil {
		return err
	}

	state.PrimaryAdminUsername = strings.TrimSpace(username)
	state.UpdatedAt = time.Now()

	if err := r.stateRepo.Update(ctx, state); err != nil {
		return err
	}
	r.setCachedState(state)
	return nil
}

// GetInstanceRules retrieves the instance rules
// Matches legacy: PK="INSTANCE#CONFIG", SK="RULES"
func (r *InstanceRepository) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	config := &models.InstanceConfig{}
	err := r.Get(ctx, storage.InstanceConfigKey, "RULES", config)

	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			// Return default instance rules if none configured
			return r.getDefaultInstanceRules(), nil
		}
		r.logger.Error("Failed to get instance rules", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "instance rules", "configuration")
	}

	// Deserialize JSON rules with validation
	if err := common.ValidateRequiredParam("rules_json", config.RulesJSON); err != nil {
		return r.getDefaultInstanceRules(), nil
	}

	var result []storage.InstanceRule
	if err := json.Unmarshal([]byte(config.RulesJSON), &result); err != nil {
		r.logger.Error("Failed to unmarshal instance rules, falling back to defaults", zap.Error(err))
		return r.getDefaultInstanceRules(), nil
	}

	// Validate and filter rules
	validatedRules := r.validateAndFilterRules(result)
	r.logger.Debug("Retrieved instance rules", zap.Int("count", len(validatedRules)))

	return validatedRules, nil
}

// SetInstanceRules updates the instance rules
// Matches legacy: assigns IDs if not present, PK="INSTANCE#CONFIG", SK="RULES"
func (r *InstanceRepository) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	// Assign IDs if not present (matches legacy behavior)
	processedRules := make([]storage.InstanceRule, len(rules))
	for i, rule := range rules {
		processedRules[i] = rule
		if err := common.ValidateRequiredParam("rule_id", processedRules[i].ID); err != nil {
			processedRules[i].ID = fmt.Sprintf("%d", i+1)
		}
	}

	// Serialize rules to JSON
	rulesJSON, err := json.Marshal(processedRules)
	if err != nil {
		r.logger.Error("Failed to marshal instance rules", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "instance rules", "configuration")
	}

	config := models.NewInstanceRulesConfig(string(rulesJSON))

	err = r.Create(ctx, config)
	if err != nil {
		r.logger.Error("Failed to save instance rules", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "instance rules", "configuration")
	}

	return nil
}

// GetExtendedDescription retrieves the instance extended description
// Matches legacy: PK="INSTANCE#CONFIG", SK="EXTENDED_DESC", returns default if not set
func (r *InstanceRepository) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	config := &models.InstanceConfig{}
	err := r.Get(ctx, storage.InstanceConfigKey, "EXTENDED_DESC", config)

	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			// Return enhanced default description with instance info
			defaultDesc := r.generateDefaultDescription()
			return defaultDesc, time.Now(), nil
		}
		r.logger.Error("Failed to get extended description", zap.Error(err))
		return "", time.Time{}, ErrorHandler.HandleGetError(err, "instance metadata", "extended description")
	}

	// Validate and sanitize the description
	sanitizedDesc := r.sanitizeDescription(config.ExtendedDescription)
	return sanitizedDesc, config.UpdatedAt, nil
}

// SetExtendedDescription updates the instance extended description
// Matches legacy: PK="INSTANCE#CONFIG", SK="EXTENDED_DESC"
func (r *InstanceRepository) SetExtendedDescription(ctx context.Context, description string) error {
	config := models.NewExtendedDescriptionConfig(description)

	err := r.Create(ctx, config)
	if err != nil {
		r.logger.Error("Failed to save extended description", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "instance metadata", "extended description")
	}

	return nil
}

// GetRulesByCategory retrieves rules filtered by category
// Since legacy doesn't implement this, we'll use the instance rules model with category filtering
func (r *InstanceRepository) GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error) {
	rules, err := r.GetInstanceRules(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by category using rule text patterns and metadata
	filtered := make([]storage.InstanceRule, 0)
	for _, rule := range rules {
		if r.ruleMatchesCategory(rule, category) {
			filtered = append(filtered, rule)
		}
	}

	// If no rules match specific category, apply smart categorization
	if len(filtered) == 0 && category != "" {
		filtered = r.categorizeRulesSmartly(rules, category)
	}

	r.logger.Debug("Filtered rules by category",
		zap.String("category", category),
		zap.Int("total_rules", len(rules)),
		zap.Int("filtered_count", len(filtered)))

	return filtered, nil
}

// GetTotalUserCount returns the total number of users
// Since legacy doesn't implement this, use instance metrics pattern
func (r *InstanceRepository) GetTotalUserCount(ctx context.Context) (int64, error) {
	metric := &models.InstanceMetrics{}
	err := r.metricsRepo.Get(ctx, "INSTANCE#METRICS", "TOTAL_USERS", metric)

	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return 0, nil
		}
		r.logger.Error("Failed to get total user count", zap.Error(err))
		return 0, ErrorHandler.HandleGetError(err, "instance metrics", "total users")
	}

	return metric.TotalUsers, nil
}

// GetTotalStatusCount returns the total number of statuses
func (r *InstanceRepository) GetTotalStatusCount(ctx context.Context) (int64, error) {
	metric := &models.InstanceMetrics{}
	err := r.metricsRepo.Get(ctx, "INSTANCE#METRICS", "TOTAL_STATUSES", metric)

	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return 0, nil
		}
		r.logger.Error("Failed to get total status count", zap.Error(err))
		return 0, ErrorHandler.HandleGetError(err, "instance metrics", "total statuses")
	}

	return metric.TotalStatuses, nil
}

// GetTotalDomainCount returns the total number of known domains
func (r *InstanceRepository) GetTotalDomainCount(ctx context.Context) (int64, error) {
	metric := &models.InstanceMetrics{}
	err := r.metricsRepo.Get(ctx, "INSTANCE#METRICS", "TOTAL_DOMAINS", metric)

	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return 0, nil
		}
		r.logger.Error("Failed to get total domain count", zap.Error(err))
		return 0, ErrorHandler.HandleGetError(err, "instance metrics", "total domains")
	}

	return metric.Value, nil
}

// GetActiveUserCount returns the number of active users in the last N days
func (r *InstanceRepository) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	metricType := fmt.Sprintf("ACTIVE_USERS_%dD", days)
	metric := &models.InstanceMetrics{}
	err := r.metricsRepo.Get(ctx, "INSTANCE#METRICS", metricType, metric)

	if err != nil {
		if appErrors.HasCode(err, appErrors.CodeNotFound) {
			return 0, nil
		}
		r.logger.Error("Failed to get active user count", zap.Error(err), zap.Int("days", days))
		return 0, ErrorHandler.HandleGetError(err, "instance metrics", fmt.Sprintf("active users %dD", days))
	}

	return metric.Value, nil
}

// GetDailyActiveUserCount returns the number of daily active users
func (r *InstanceRepository) GetDailyActiveUserCount(ctx context.Context) (int64, error) {
	return r.GetActiveUserCount(ctx, 1)
}

// GetLocalPostCount returns the number of local posts
func (r *InstanceRepository) GetLocalPostCount(ctx context.Context) (int64, error) {
	metric := &models.InstanceMetrics{}
	err := r.metricsRepo.Get(ctx, "INSTANCE#METRICS", "LOCAL_POSTS", metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		r.logger.Error("Failed to get local post count", zap.Error(err))
		return 0, ErrorHandler.HandleGetError(err, "instance metrics", "local posts")
	}

	return metric.Value, nil
}

// GetLocalCommentCount returns the number of local comments (posts with InReplyToID)
func (r *InstanceRepository) GetLocalCommentCount(ctx context.Context) (int64, error) {
	metric := &models.InstanceMetrics{}
	err := r.metricsRepo.Get(ctx, "INSTANCE#METRICS", "LOCAL_COMMENTS", metric)

	if err != nil {
		if errors.IsNotFound(err) {
			// Scan-free: this metric is maintained in real time on status create/delete.
			// If it's missing (new deploy), return 0 and allow a one-time backfill tool to seed it.
			r.logger.Warn("LOCAL_COMMENTS metric missing; returning 0 (run backfill to seed)")
			return 0, nil
		}
		r.logger.Error("Failed to get local comment count", zap.Error(err))
		return 0, ErrorHandler.HandleGetError(err, "instance metrics", "local comments")
	}

	return metric.Value, nil
}

// GetWeeklyActivity retrieves weekly activity data for a specific week
func (r *InstanceRepository) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	activity := &models.WeeklyActivity{}

	// Use the pattern from the model: PK="INSTANCE#ACTIVITY", SK="ACTIVITY#WEEK#{date}"
	weekStart := time.Unix(weekTimestamp, 0).Format(common.DateFormat)
	err := r.activityRepo.Get(ctx, "INSTANCE#ACTIVITY", fmt.Sprintf("ACTIVITY#WEEK#%s", weekStart), activity)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "instance activity", fmt.Sprintf("week %d", weekTimestamp))
		}
		r.logger.Error("Failed to get weekly activity", zap.Error(err), zap.Int64("week", weekTimestamp))
		return nil, ErrorHandler.HandleGetError(err, "instance activity", fmt.Sprintf("week %d", weekTimestamp))
	}

	return &storage.WeeklyActivity{
		Week:          fmt.Sprintf("%d", activity.Week),
		Statuses:      int(activity.Statuses),
		Logins:        int(activity.Logins),
		Registrations: int(activity.Registrations),
	}, nil
}

// RecordActivity records activity data for analytics
func (r *InstanceRepository) RecordActivity(ctx context.Context, activityType string, _ string, timestamp time.Time) error {
	// Create activity record for instance-wide tracking
	week := getWeekStart(timestamp)

	// Create a new weekly activity using the model's constructor
	activity := models.NewWeeklyActivity(week)

	// Try to get existing record first
	err := r.activityRepo.Get(ctx, activity.PK, activity.SK, activity)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("Failed to get existing weekly activity", zap.Error(err))
		return ErrorHandler.HandleGetError(err, "instance activity", fmt.Sprintf("week %s", week.Format("2006-01-02")))
	}

	// Update activity counters based on type
	switch strings.ToLower(activityType) {
	case "status", "post":
		activity.IncrementStatuses(1)
	case "login":
		activity.IncrementLogins(1)
	case "registration", "signup":
		activity.IncrementRegistrations(1)
	}

	// Save the updated activity
	err = r.activityRepo.Create(ctx, activity)
	if err != nil {
		r.logger.Error("Failed to record activity", zap.Error(err), zap.String("type", activityType))
		return ErrorHandler.HandleCreateError(err, "instance activity", activityType)
	}

	return nil
}

// GetContactAccount returns the contact account for the instance
// This returns the first admin user as the contact account
func (r *InstanceRepository) GetContactAccount(ctx context.Context) (*storage.ActorRecord, error) {
	// Look for the first admin user to serve as contact account
	var users []models.User
	err := r.metricsRepo.GetDB().WithContext(ctx).Model(&models.User{}).
		Index("gsi3").
		Where("gsi3PK", "=", "ROLE#admin").
		Limit(1).
		All(&users)

	if err != nil {
		r.logger.Error("Failed to query admin users for contact account", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "instance", "admin users")
	}

	if err := common.ValidateSliceNotEmpty("users", users); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityActor, "admin contact")
	}

	user := users[0]

	// Get the corresponding actor for this user
	var actor models.Actor
	err = r.metricsRepo.GetDB().WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s", user.Username)).
		Where("SK", "=", "PROFILE").
		First(&actor)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityActor, user.Username)
		}
		r.logger.Error("Failed to get actor for contact account",
			zap.String("username", user.Username),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "instance", "contact actor")
	}

	// Convert the actor model to storage.ActorRecord format
	actorRecord := &storage.ActorRecord{
		PK:       actor.PK,
		SK:       actor.SK,
		Username: actor.Username,
		// PrivateKey is not included for security reasons when returning contact info
	}

	return actorRecord, nil
}

// GetStorageUsage returns current storage usage statistics
func (r *InstanceRepository) GetStorageUsage(ctx context.Context) (any, error) {
	metric := &models.InstanceMetrics{}
	err := r.metricsRepo.Get(ctx, "INSTANCE#METRICS", "STORAGE_USAGE", metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return map[string]interface{}{
				"total_bytes": 0,
				"media_bytes": 0,
				"db_bytes":    0,
			}, nil
		}
		r.logger.Error("Failed to get storage usage", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "instance metrics", "storage usage")
	}

	return map[string]interface{}{
		"total_bytes": metric.Value,
		"updated_at":  metric.UpdatedAt,
	}, nil
}

// getMetricHistory is a consolidated helper that retrieves history for different metric types
func (r *InstanceRepository) getMetricHistory(ctx context.Context, days int, metricType, operation string, formatter func(models.InstanceHistory) map[string]interface{}) ([]any, error) {
	// Direct implementation since we don't have BaseRepository embedded
	if err := common.ValidateIntRange("days", days, 1, 365); err != nil {
		days = 30 // Default to 30 days
	}

	// Calculate date range
	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	endDate := time.Now().Format("2006-01-02")

	// Query daily metrics using GSI1
	var histories []models.InstanceHistory
	err := r.historyRepo.GetDB().WithContext(ctx).Model(&models.InstanceHistory{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("METRIC#%s", metricType)).
		Where("gsi1SK", ">=", fmt.Sprintf("DATE#%s", startDate)).
		Where("gsi1SK", "<=", fmt.Sprintf("DATE#%s", endDate)).
		All(&histories)

	if err != nil {
		r.logger.Error(fmt.Sprintf("Failed to get %s", operation), zap.Error(err), zap.Int("days", days))
		return nil, ErrorHandler.HandleQueryError(err, "instance metrics", operation)
	}

	// Convert to expected format using the provided formatter
	result := make([]any, len(histories))
	for i, h := range histories {
		result[i] = formatter(h)
	}

	r.logger.Info(fmt.Sprintf("Retrieved %s", operation), zap.Int("days", days), zap.Int("records", len(result)))
	return result, nil
}

// GetStorageHistory returns storage usage history for the last N days
func (r *InstanceRepository) GetStorageHistory(ctx context.Context, days int) ([]any, error) {
	return r.getMetricHistory(ctx, days, "storage_bytes", "storage history", func(h models.InstanceHistory) map[string]interface{} {
		return map[string]interface{}{
			"date":           h.Date,
			"total_bytes":    h.StorageBytes,
			"media_bytes":    h.MediaBytes,
			"database_bytes": h.DatabaseBytes,
			"delta":          h.Delta,
			"recorded_at":    h.RecordedAt,
		}
	})
}

// GetUserGrowthHistory returns user growth data for the last N days
func (r *InstanceRepository) GetUserGrowthHistory(ctx context.Context, days int) ([]any, error) {
	return r.getMetricHistory(ctx, days, "user_count", "user growth history", func(h models.InstanceHistory) map[string]interface{} {
		return map[string]interface{}{
			"date":         h.Date,
			"total_users":  h.TotalUsers,
			"active_users": h.ActiveUsers,
			"new_users":    h.NewUsers,
			"delta":        h.Delta,
			"recorded_at":  h.RecordedAt,
		}
	})
}

// GetDomainStats returns statistics for a specific domain
func (r *InstanceRepository) GetDomainStats(ctx context.Context, domain string) (any, error) {
	metric := &models.InstanceMetrics{}
	err := r.metricsRepo.Get(ctx, fmt.Sprintf("DOMAIN#%s", domain), "STATS", metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return map[string]interface{}{
				"domain":        domain,
				"actor_count":   0,
				"status_count":  0,
				"last_activity": nil,
			}, nil
		}
		r.logger.Error("Failed to get domain stats", zap.Error(err), zap.String("domain", domain))
		return nil, ErrorHandler.HandleGetError(err, "instance metrics", fmt.Sprintf("domain %s", domain))
	}

	return map[string]interface{}{
		"domain":        domain,
		"actor_count":   metric.Value,
		"last_activity": metric.UpdatedAt,
	}, nil
}

// RecordDailyMetrics records daily historical metrics for the instance
func (r *InstanceRepository) RecordDailyMetrics(ctx context.Context, date string, metrics map[string]interface{}) error {
	now := time.Now()
	if err := common.ValidateRequiredParam("date", date); err != nil {
		date = now.Format("2006-01-02")
	}

	// Record user metrics
	if userCount, ok := metrics["total_users"].(int64); ok {
		userHistory := models.NewDailyInstanceHistory(date, "user_count")
		if activeUsers, hasActive := metrics["active_users"].(int64); hasActive {
			if newUsers, hasNew := metrics["new_users"].(int64); hasNew {
				userHistory.SetUserMetrics(userCount, activeUsers, newUsers)
			} else {
				userHistory.SetUserMetrics(userCount, activeUsers, 0)
			}
		} else {
			userHistory.SetUserMetrics(userCount, 0, 0)
		}

		// Get previous day's value for delta calculation
		if prevValue, err := r.getPreviousDayValue(ctx, date, "user_count"); err == nil {
			userHistory.CalculateDelta(prevValue)
		}

		if err := r.historyRepo.Create(ctx, userHistory); err != nil {
			r.logger.Error("Failed to record daily user metrics", zap.Error(err), zap.String("date", date))
			return ErrorHandler.HandleCreateError(err, "instance metrics", "user metrics")
		}
	}

	// Record storage metrics
	if storageBytes, ok := metrics["storage_bytes"].(int64); ok {
		storageHistory := models.NewDailyInstanceHistory(date, "storage_bytes")
		mediaBytes, _ := metrics["media_bytes"].(int64)
		dbBytes, _ := metrics["database_bytes"].(int64)
		storageHistory.SetStorageMetrics(storageBytes, mediaBytes, dbBytes)

		// Get previous day's value for delta calculation
		if prevValue, err := r.getPreviousDayValue(ctx, date, "storage_bytes"); err == nil {
			storageHistory.CalculateDelta(prevValue)
		}

		if err := r.historyRepo.Create(ctx, storageHistory); err != nil {
			r.logger.Error("Failed to record daily storage metrics", zap.Error(err), zap.String("date", date))
			return ErrorHandler.HandleCreateError(err, "instance metrics", "storage metrics")
		}
	}

	// Record post metrics
	if postCount, ok := metrics["total_posts"].(int64); ok {
		postHistory := models.NewDailyInstanceHistory(date, "post_count")
		newPosts, _ := metrics["new_posts"].(int64)
		localPosts, _ := metrics["local_posts"].(int64)
		federatedPosts, _ := metrics["federated_posts"].(int64)
		postHistory.SetPostMetrics(postCount, newPosts, localPosts, federatedPosts)

		// Get previous day's value for delta calculation
		if prevValue, err := r.getPreviousDayValue(ctx, date, "post_count"); err == nil {
			postHistory.CalculateDelta(prevValue)
		}

		if err := r.historyRepo.Create(ctx, postHistory); err != nil {
			r.logger.Error("Failed to record daily post metrics", zap.Error(err), zap.String("date", date))
			return ErrorHandler.HandleCreateError(err, "instance metrics", "post metrics")
		}
	}

	// Record federation metrics
	if knownInstances, ok := metrics["known_instances"].(int64); ok {
		fedHistory := models.NewDailyInstanceHistory(date, "federation_count")
		activeInstances, _ := metrics["active_instances"].(int64)
		fedHistory.SetFederationMetrics(knownInstances, activeInstances)

		// Get previous day's value for delta calculation
		if prevValue, err := r.getPreviousDayValue(ctx, date, "federation_count"); err == nil {
			fedHistory.CalculateDelta(prevValue)
		}

		if err := r.historyRepo.Create(ctx, fedHistory); err != nil {
			r.logger.Error("Failed to record daily federation metrics", zap.Error(err), zap.String("date", date))
			return ErrorHandler.HandleCreateError(err, "instance metrics", "federation metrics")
		}
	}

	r.logger.Info("Successfully recorded daily metrics", zap.String("date", date))
	return nil
}

// GetMetricsSummary returns aggregated metrics for a given time range
func (r *InstanceRepository) GetMetricsSummary(ctx context.Context, timeRange string) (map[string]interface{}, error) {
	var startDate, endDate string
	now := time.Now()

	switch timeRange {
	case "week":
		startDate = now.AddDate(0, 0, -7).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	case "month":
		startDate = now.AddDate(0, -1, 0).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	case "quarter":
		startDate = now.AddDate(0, -3, 0).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	case "year":
		startDate = now.AddDate(-1, 0, 0).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	default:
		startDate = now.AddDate(0, 0, -30).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	summary := make(map[string]interface{})

	// Get metrics for each type
	metricTypes := []string{"user_count", "storage_bytes", "post_count", "federation_count"}

	for _, metricType := range metricTypes {
		var histories []models.InstanceHistory
		err := r.historyRepo.GetDB().WithContext(ctx).Model(&models.InstanceHistory{}).
			Index("gsi1").
			Where("gsi1PK", "=", fmt.Sprintf("METRIC#%s", metricType)).
			Where("gsi1SK", ">=", fmt.Sprintf("DATE#%s", startDate)).
			Where("gsi1SK", "<=", fmt.Sprintf("DATE#%s", endDate)).
			All(&histories)

		if err != nil {
			r.logger.Error("Failed to get metrics summary", zap.Error(err), zap.String("metric_type", metricType))
			continue
		}

		if err := common.ValidateSliceNotEmpty("histories", histories); err == nil {
			// Get latest and earliest values for growth calculation
			latest := histories[len(histories)-1]
			earliest := histories[0]
			growth := float64(0)
			if earliest.Value > 0 {
				growth = ((float64(latest.Value) - float64(earliest.Value)) / float64(earliest.Value)) * 100
			}

			summary[metricType] = map[string]interface{}{
				"current_value": latest.Value,
				"start_value":   earliest.Value,
				"growth_pct":    growth,
				"total_change":  latest.Value - earliest.Value,
				"data_points":   len(histories),
			}
		}
	}

	summary["time_range"] = timeRange
	summary["start_date"] = startDate
	summary["end_date"] = endDate
	summary["generated_at"] = time.Now()

	return summary, nil
}

// getPreviousDayValue gets the value from the previous day for delta calculation
func (r *InstanceRepository) getPreviousDayValue(ctx context.Context, currentDate, metricType string) (int64, error) {
	// Parse current date and get previous day
	date, err := time.Parse("2006-01-02", currentDate)
	if err != nil {
		return 0, err
	}
	prevDate := date.AddDate(0, 0, -1).Format("2006-01-02")

	history := &models.InstanceHistory{}
	err = r.historyRepo.GetDB().WithContext(ctx).Model(&models.InstanceHistory{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("METRIC#%s", metricType)).
		Where("gsi1SK", "=", fmt.Sprintf("DATE#%s", prevDate)).
		First(history)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil // No previous data, delta from 0
		}
		return 0, err
	}

	return history.Value, nil
}

// Helper function to get the start of the week for a given timestamp
func getWeekStart(t time.Time) time.Time {
	// Get Monday of the week
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	return t.AddDate(0, 0, -(weekday - 1)).Truncate(24 * time.Hour)
}

// getDefaultInstanceRules returns a set of default rules when none are configured
func (r *InstanceRepository) getDefaultInstanceRules() []storage.InstanceRule {
	return []storage.InstanceRule{
		{
			ID:   "1",
			Text: "Be respectful and kind to other users",
		},
		{
			ID:   "2",
			Text: "No harassment, hate speech, or discrimination",
		},
		{
			ID:   "3",
			Text: "No spam or excessive promotional content",
		},
		{
			ID:   "4",
			Text: "Use appropriate content warnings for sensitive material",
		},
		{
			ID:   "5",
			Text: "Follow local and international laws",
		},
	}
}

// validateAndFilterRules validates rules and removes invalid ones
func (r *InstanceRepository) validateAndFilterRules(rules []storage.InstanceRule) []storage.InstanceRule {
	validated := make([]storage.InstanceRule, 0, len(rules))
	seenIDs := make(map[string]bool)

	for i, rule := range rules {
		// Ensure rule has an ID
		if err := common.ValidateRequiredParam("rule_id", rule.ID); err != nil {
			rule.ID = fmt.Sprintf("rule_%d", i+1)
		}

		// Check for duplicate IDs
		if seenIDs[rule.ID] {
			// Generate unique ID for duplicates
			rule.ID = fmt.Sprintf("%s_dup_%d", rule.ID, i)
		}
		seenIDs[rule.ID] = true

		// Validate rule text
		if err := common.ValidateRequiredParam("rule_text", strings.TrimSpace(rule.Text)); err != nil {
			r.logger.Warn("Skipping rule with empty text", zap.String("id", rule.ID))
			continue
		}

		// Limit text length
		if len(rule.Text) > 500 {
			rule.Text = rule.Text[:497] + "..."
		}

		validated = append(validated, rule)
	}

	return validated
}

// generateDefaultDescription creates a dynamic default description
func (r *InstanceRepository) generateDefaultDescription() string {
	return fmt.Sprintf(`<div class="instance-description">
		<h2>Welcome to Lesser</h2>
		<p>This is a Lesser ActivityPub server, part of the decentralized social web.</p>
		
		<h3>About Lesser</h3>
		<p>Lesser is a lightweight, cost-effective ActivityPub implementation designed for 
		personal and small community use. It provides full compatibility with Mastodon and 
		other fediverse applications while maintaining minimal operational costs.</p>
		
		<h3>Features</h3>
		<ul>
			<li>Full Mastodon API compatibility</li>
			<li>ActivityPub federation</li>
			<li>WebSocket streaming</li>
			<li>GraphQL API</li>
			<li>Cost-optimized serverless architecture</li>
		</ul>
		
		<p><em>Generated on %s</em></p>
	</div>`, time.Now().Format("2006-01-02"))
}

// sanitizeDescription sanitizes HTML content in descriptions
func (r *InstanceRepository) sanitizeDescription(desc string) string {
	// Basic HTML sanitization - in production, use a proper HTML sanitizer
	desc = strings.ReplaceAll(desc, "<script", "&lt;script")
	desc = strings.ReplaceAll(desc, "</script>", "&lt;/script&gt;")
	desc = strings.ReplaceAll(desc, "javascript:", "")
	desc = strings.ReplaceAll(desc, "on=", "data-on=")

	// Limit length
	if err := common.ValidateStringLength("description", desc, 0, 10000); err != nil {
		desc = desc[:9997] + "..."
	}

	return desc
}

// ruleMatchesCategory checks if a rule matches a given category
func (r *InstanceRepository) ruleMatchesCategory(rule storage.InstanceRule, category string) bool {
	if err := common.ValidateRequiredParam("category", category); err != nil {
		return true // Return all rules for empty category
	}

	ruleTextLower := strings.ToLower(rule.Text)
	categoryLower := strings.ToLower(category)

	// Define category keywords
	categoryKeywords := map[string][]string{
		"harassment": {"harassment", "abuse", "bullying", "threatening", "intimidation"},
		"spam":       {"spam", "promotional", "advertising", "solicitation", "flooding"},
		"content":    {"content warning", "nsfw", "sensitive", "explicit", "graphic"},
		"legal":      {"illegal", "law", "legal", "copyright", "dmca", "piracy"},
		"conduct":    {"respectful", "kind", "civil", "behavior", "conduct", "etiquette"},
		"hate":       {"hate speech", "discrimination", "racism", "sexism", "homophobia"},
		"privacy":    {"privacy", "personal info", "doxxing", "private", "confidential"},
	}

	keywords, exists := categoryKeywords[categoryLower]
	if !exists {
		// Try partial matching for unknown categories
		return strings.Contains(ruleTextLower, categoryLower)
	}

	// Check if any keywords match
	for _, keyword := range keywords {
		if strings.Contains(ruleTextLower, keyword) {
			return true
		}
	}

	return false
}

// categorizeRulesSmartly applies intelligent categorization when no direct matches
func (r *InstanceRepository) categorizeRulesSmartly(rules []storage.InstanceRule, category string) []storage.InstanceRule {
	// If requesting a specific category but no matches, apply fuzzy logic
	filtered := make([]storage.InstanceRule, 0)
	categoryLower := strings.ToLower(category)

	// For unknown categories, do fuzzy text matching
	for _, rule := range rules {
		ruleTextLower := strings.ToLower(rule.Text)

		// Calculate similarity score (simple implementation)
		if r.calculateSimilarity(ruleTextLower, categoryLower) > 0.3 {
			filtered = append(filtered, rule)
		}
	}

	// If still no matches, return most relevant rules based on common sense
	if err := common.ValidateSliceNotEmpty("filtered", filtered); err != nil {
		switch categoryLower {
		case "safety", "security":
			// Return rules about harassment, abuse, etc.
			for _, rule := range rules {
				if r.ruleMatchesCategory(rule, "harassment") || r.ruleMatchesCategory(rule, "hate") {
					filtered = append(filtered, rule)
				}
			}
		case "posting", "content":
			// Return rules about content guidelines
			for _, rule := range rules {
				if r.ruleMatchesCategory(rule, "content") || r.ruleMatchesCategory(rule, "spam") {
					filtered = append(filtered, rule)
				}
			}
		default:
			// Return top 3 most important rules
			if err := common.ValidateSliceLength("rules", rules, 3); err != nil {
				filtered = rules[:3]
			} else {
				filtered = rules
			}
		}
	}

	return filtered
}

// calculateSimilarity calculates a simple text similarity score (0.0 to 1.0)
func (r *InstanceRepository) calculateSimilarity(text1, text2 string) float64 {
	words1 := strings.Fields(text1)
	words2 := strings.Fields(text2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	matches := 0
	for _, word1 := range words1 {
		for _, word2 := range words2 {
			if strings.Contains(word1, word2) || strings.Contains(word2, word1) {
				matches++
				break
			}
		}
	}

	// Return ratio of matching words to total unique words
	totalWords := len(words1) + len(words2)
	return float64(matches*2) / float64(totalWords)
}
